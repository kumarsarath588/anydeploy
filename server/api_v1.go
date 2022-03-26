package apiv1

import (
	rabbitmq "anydeploy/amqp"
	"anydeploy/db"
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

const (
	PENDING                    = "PENDING"
	CREATING_DEPLOYMENT        = "CREATING_DEPLOYMENT"
	CREATED_DEPLOYMENT         = "CREATED_DEPLOYMENT"
	CREATING_PUBLISHED_SERVICE = "CREATING_PUBLISHED_SERVICE"
	CREATED_PUBLISHED_SERVICE  = "CREATED_PUBLISHED_SERVICE"
	SUCCESS                    = "SUCCESS"
	FAILED                     = "FAILED"
	DELETING                   = "DELETING"
	DELETING_DEPLOYMENT        = "DELETING_DEPLOYMENT"
	DELETED_DEPLOYMENT         = "DELETED_DEPLOYMENT"
	DELETING_PUBLISHED_SERVICE = "DELETING_PUBLISHED_SERVICE"
	DELETED_PUBLISHED_SERVICE  = "DELETING_PUBLISHED_SERVICE"
	DELETED                    = "DELETED"
)

const tableCreationQuery = `CREATE TABLE IF NOT EXISTS anydeploy.deployments (
    uuid VARCHAR(255) NOT NULL,
	name VARCHAR(255) NOT NULL,
	type VARCHAR(255) NOT NULL,
	state VARCHAR(255) NOT NULL,
	payload VARCHAR(16383) NOT NULL,
    PRIMARY KEY (uuid)
);`

type Port struct {
	Name string `json:"name,omitempty"`
	Port int    `json:"port,omitempty"`
}

type Container struct {
	Name  string  `json:"name,omitempty"`
	Image string  `json:"image,omitempty"`
	Tag   string  `json:"image_tag,omitempty"`
	Ports []*Port `json:"ports,omitempty"`
}
type ProvisionSpec struct {
	Name                 string       `json:"name,omitempty"`
	Type                 string       `json:"type,omitempty"`
	Contianers           []*Container `json:"containers,omitempty"`
	PublishedServiceType string       `json:"published_service_type,omitempty"`
	Expose               int          `json:"expose,omitempty"`
}

type ProvisionStatus struct {
	State      string `json:"state,omitempty"`
	Message    string `json:"message,omitempty"`
	ExternalIP string `json:"external_ip,omitempty"`
}

type ProvisionMetadata struct {
	UUID string `json:"uuid,omitempty"`
}

type Provision struct {
	Spec     *ProvisionSpec     `json:"spec,omitempty"`
	Status   *ProvisionStatus   `json:"status,omitempty"`
	Metadata *ProvisionMetadata `json:"metadata,omitempty"`
}

type healthCheckResponse struct {
	Status string `json:"status"`
}

func IsValidUUID(u string) bool {
	_, err := uuid.Parse(u)
	return err == nil
}

type App struct {
	Router *mux.Router
	DB     *sql.DB
	AMQP   *rabbitmq.Session
}

func (a *App) Initialize(user, password, host, port, dbname, amqp_connection, queue_name string) {
	log.Printf("Initalizing database with user '%s', host '%s', port '%s', dbname '%s'", user, host, port, dbname)
	connectionString :=
		fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", user, password, host, port, dbname)

	var err error
	a.DB, err = sql.Open("mysql", connectionString)
	if err != nil {
		log.Fatal(err)
	}

	a.createTable()

	a.InitializeAMQP(queue_name, amqp_connection)

	a.Router = mux.NewRouter()

	a.initializeRoutes()
}

func (a *App) InitializeAMQP(name string, addr string) {
	a.AMQP = rabbitmq.New(name, addr)
	time.Sleep(time.Second * 5)
}

func (a *App) createTable() {
	log.Printf("Executing db migrate")
	if _, err := a.DB.Exec(tableCreationQuery); err != nil {
		log.Fatal(err)
	}
}

func (a *App) Run(addr string) {
	loggedRouter := handlers.LoggingHandler(os.Stdout, a.Router)
	compressedRouter := handlers.CompressHandler(loggedRouter)
	log.Fatal(http.ListenAndServe(addr, compressedRouter))
}

func (a *App) initializeRoutes() {
	log.Printf("Initializing routes")
	a.Router.HandleFunc("/api/v1/provision", a.NewProvisionRequest).Methods("POST")
	a.Router.HandleFunc("/api/v1/provision/{uuid}", a.GetProvisionRequest).Methods("GET")
	a.Router.HandleFunc("/api/v1/provision/{uuid}", a.DeleteProvisionRequest).Methods("DELETE")
	a.Router.HandleFunc("/health", a.HealthCheck).Methods("GET")
	a.Router.NotFoundHandler = http.HandlerFunc(notFoundHandler)

}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "404 - Not Found!", http.StatusNotFound)
}

func writeJsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Encoding", "gzip")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(&data)
}

func (a *App) HealthCheck(w http.ResponseWriter, r *http.Request) {

	err := a.DB.Ping()
	if err != nil {
		log.Printf("DB conneciton is not ok %s", err.Error())
	} else {
		log.Printf("DB conneciton is ok")
	}

	if err != nil {
		data := &healthCheckResponse{Status: "Database unaccessible"}
		writeJsonResponse(w, http.StatusServiceUnavailable, data)
	} else {
		data := &healthCheckResponse{Status: "OK"}
		writeJsonResponse(w, http.StatusOK, data)
	}
}

func encodeToBase64(v interface{}) (string, error) {
	var buf bytes.Buffer
	encoder := base64.NewEncoder(base64.StdEncoding, &buf)
	err := json.NewEncoder(encoder).Encode(v)
	if err != nil {
		return "", err
	}
	encoder.Close()
	return buf.String(), nil
}

func decodeFromBase64(v interface{}, enc string) error {
	return json.NewDecoder(base64.NewDecoder(base64.StdEncoding, strings.NewReader(enc))).Decode(v)
}

func setStatus(message string, state string) *ProvisionStatus {
	return &ProvisionStatus{
		Message: message,
		State:   state,
	}
}

func PushMessage(queue *rabbitmq.Session, message string) error {

	byte_message := []byte(message)

	if err := queue.Push(byte_message); err != nil {
		return fmt.Errorf("push to amqp failed: %s", err)
	} else {
		return nil
	}

}

func (a *App) NewProvisionRequest(w http.ResponseWriter, r *http.Request) {
	var payload Provision

	reqBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		payload.Status = setStatus("Invalid json data provided", FAILED)
		writeJsonResponse(w, http.StatusBadRequest, &payload)
		return
	}

	json.Unmarshal(reqBody, &payload)

	if payload.Spec == nil {
		payload.Status = setStatus("Invalid json data provided", FAILED)
		writeJsonResponse(w, http.StatusBadRequest, &payload)
		return
	}

	if payload.Metadata == nil {
		id := uuid.New()
		payload.Metadata = &ProvisionMetadata{
			UUID: id.String(),
		}
	}

	if !IsValidUUID(payload.Metadata.UUID) {
		payload.Status = setStatus(
			fmt.Sprintf("uuid %s is invalid", payload.Metadata.UUID), FAILED)
		writeJsonResponse(w, http.StatusBadRequest, &payload)
		return
	}

	// Doing basic validation; can use yaml spec to do advanced validation of received json payload
	// No prechecks to see of provision request with same name exists or not
	if payload.Spec.Name == "" || payload.Spec.Type == "" {
		payload.Status = setStatus("Both Spec.Name and Spec.Type are required.", FAILED)
		writeJsonResponse(w, http.StatusBadRequest, &payload)
		return
	}

	if len(payload.Spec.Contianers) == 0 {
		payload.Status = setStatus("Please add atleast one container for provision", FAILED)
		writeJsonResponse(w, http.StatusBadRequest, &payload)
		return
	}

	payload.Status = &ProvisionStatus{
		State: PENDING,
	}

	encoded_payload, err := encodeToBase64(payload)
	if err != nil {
		payload.Status = setStatus("Failed to provision", FAILED)
		writeJsonResponse(w, http.StatusBadRequest, &payload)
		log.Printf("Failed to encode data of payload.")
		return
	}

	db_payload := &db.DBPayload{
		UUID:   payload.Metadata.UUID,
		Name:   payload.Spec.Name,
		Type:   payload.Spec.Type,
		State:  PENDING,
		Base64: encoded_payload,
	}

	err = db.InsertEntry(a.DB, db_payload)
	if err != nil {
		payload.Status = setStatus(err.Error(), FAILED)
		writeJsonResponse(w, http.StatusBadRequest, &payload)
	} else {
		err = PushMessage(a.AMQP, encoded_payload)
		if err != nil {
			payload.Status = setStatus(err.Error(), FAILED)
			writeJsonResponse(w, http.StatusBadRequest, &payload)
		} else {
			writeJsonResponse(w, http.StatusCreated, &payload)
		}
	}
}

func (a *App) GetProvisionRequest(w http.ResponseWriter, r *http.Request) {

	var payload Provision

	vars := mux.Vars(r)
	uuid := vars["uuid"]

	if !IsValidUUID(uuid) {
		payload.Status = setStatus(
			fmt.Sprintf("Invalid uuid %s", uuid), FAILED)
		writeJsonResponse(w, http.StatusBadRequest, &payload)
		return
	}

	provision_request, err := db.GetEntry(a.DB, uuid)
	if err != nil {
		payload.Status = setStatus(
			fmt.Sprintf("Provision request with %s does not exist", uuid), FAILED)
		log.Print(err.Error())
	}

	var output_payload *Provision
	if err := decodeFromBase64(&output_payload, provision_request.Base64); err != nil {
		payload.Status = setStatus("Failed to get decode request", FAILED)
		log.Fatal(err)
	}
	if output_payload != nil {
		writeJsonResponse(w, http.StatusOK, &output_payload)
	} else {
		payload.Status = setStatus("Failed to get request", FAILED)
		writeJsonResponse(w, http.StatusNotFound, &payload)
	}
}

func (a *App) DeleteProvisionRequest(w http.ResponseWriter, r *http.Request) {

	var payload Provision

	vars := mux.Vars(r)
	uuid := vars["uuid"]

	if !IsValidUUID(uuid) {
		payload.Status = setStatus(
			fmt.Sprintf("Invalid uuid %s", uuid), FAILED)
		writeJsonResponse(w, http.StatusBadRequest, &payload)
		return
	}

	provision_request, err := db.GetEntry(a.DB, uuid)
	if err != nil {
		payload.Status = setStatus(
			fmt.Sprintf("Provision request with %s does not exist", uuid), FAILED)
		log.Print(err.Error())
	}

	var output_payload *Provision
	if err := decodeFromBase64(&output_payload, provision_request.Base64); err != nil {
		payload.Status = setStatus("Failed to get decode request", FAILED)
		log.Fatal(err)
	}

	//Set provision.Status to DELETING STATE
	output_payload.Status.State = DELETING

	encoded_payload, err := encodeToBase64(output_payload)
	if err != nil {
		payload.Status = setStatus("Failed to provision", FAILED)
		writeJsonResponse(w, http.StatusBadRequest, &payload)
		log.Println("Failed to encode data of payload.")
		return
	}

	db_payload := &db.DBPayload{
		UUID:   output_payload.Metadata.UUID,
		Name:   output_payload.Spec.Name,
		Type:   output_payload.Spec.Type,
		State:  DELETING,
		Base64: encoded_payload,
	}

	err = db.UpdateEntry(a.DB, db_payload)
	if err != nil {
		payload.Status = setStatus(
			fmt.Sprintf("unable to updated db for uuid: %s", payload.Metadata.UUID), FAILED)
		writeJsonResponse(w, http.StatusBadRequest, &payload)
		log.Printf("unable to updated db for uuid: %s", payload.Metadata.UUID)
		return
	}

	err = PushMessage(a.AMQP, encoded_payload)
	if err != nil {
		payload.Status = setStatus(err.Error(), FAILED)
		writeJsonResponse(w, http.StatusBadRequest, &output_payload)
	} else {
		writeJsonResponse(w, http.StatusCreated, &output_payload)
	}
}
