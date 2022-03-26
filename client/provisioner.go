package provisioner

import (
	rabbitmq "anydeploy/amqp"
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
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
	DELETED_PUBLISHED_SERVICE  = "DELETED_PUBLISHED_SERVICE"
	DELETED                    = "DELETED"
)

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
	Containers           []*Container `json:"containers,omitempty"`
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
	Spec     *ProvisionSpec     `json:"spec"`
	Status   *ProvisionStatus   `json:"status"`
	Metadata *ProvisionMetadata `json:"metadata"`
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

	a.InitializeAMQP(queue_name, amqp_connection)

}

func (a *App) InitializeAMQP(name string, addr string) {
	a.AMQP = rabbitmq.New(name, addr)
	time.Sleep(time.Second * 5)
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

func (a *App) ExecuteMessage(message string) {
	var payload *Provision
	if err := decodeFromBase64(&payload, message); err != nil {
		log.Fatal(err)
	}
	switch {
	case payload.Status.State == PENDING:
		err := a.createDeployment(payload)
		if err != nil {
			a.failed(payload, err)
		}
	case payload.Status.State == CREATED_DEPLOYMENT:
		err := a.createPublishedService(payload)
		if err != nil {
			a.failed(payload, err)
		}
	case payload.Status.State == CREATED_PUBLISHED_SERVICE:
		err := a.success(payload)
		if err != nil {
			a.failed(payload, err)
		}
	case payload.Status.State == DELETING:
		err := a.deleteDeployment(payload)
		if err != nil {
			a.failed(payload, err)
		}
	case payload.Status.State == DELETED_DEPLOYMENT:
		err := a.deletePublishedService(payload)
		if err != nil {
			a.failed(payload, err)
		}
	case payload.Status.State == DELETED_PUBLISHED_SERVICE:
		err := a.deleted(payload)
		if err != nil {
			a.failed(payload, err)
		}
	default:
		log.Println(fmt.Sprintf("invalid State %s", payload.Status.State))
	}
}

func (a *App) Run() {

	msgs, _ := a.AMQP.Stream()

	forever := make(chan bool)

	go func() {
		for d := range msgs {
			a.ExecuteMessage(string(d.Body))
			d.Ack(true)
		}
	}()

	log.Printf(" [*] Waiting for messages. To exit press CTRL+C")
	<-forever
}
