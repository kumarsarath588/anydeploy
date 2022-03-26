package provisioner

import (
	rabbitmq "anydeploy/amqp"
	"anydeploy/db"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

func PushMessage(queue *rabbitmq.Session, message string) error {

	byte_message := []byte(message)

	if err := queue.Push(byte_message); err != nil {
		return fmt.Errorf("push to amqp failed: %s", err)
	} else {
		return nil
	}

}

func (a *App) executorUpdateDB(payload *Provision, state string) error {

	payload.Status.State = state

	encoded_payload, err := encodeToBase64(payload)
	if err != nil {
		return err
	}

	db_payload := &db.DBPayload{
		UUID:   payload.Metadata.UUID,
		Name:   payload.Spec.Name,
		Type:   payload.Spec.Type,
		State:  payload.Status.State,
		Base64: encoded_payload,
	}
	err = db.UpdateEntry(a.DB, db_payload)
	if err != nil {
		return fmt.Errorf("unable to updated db for uuid: %s", payload.Metadata.UUID)
	}

	return nil
}

func (a *App) createDeployment(payload *Provision) error {
	currentState := CREATING_DEPLOYMENT
	nextState := CREATED_DEPLOYMENT

	err := a.executorUpdateDB(payload, currentState)
	if err != nil {
		return err
	}

	err = CreateKubernetesDeployment(payload.Metadata.UUID, payload.Spec.Name, payload.Spec.Containers)
	if err != nil {
		return err
	}

	err = a.executorUpdateDB(payload, nextState)
	if err != nil {
		return err
	}

	encoded_payload, err := encodeToBase64(payload)
	if err != nil {
		return err
	}

	err = PushMessage(a.AMQP, encoded_payload)
	if err != nil {
		return err
	}

	return nil
}

func (a *App) createPublishedService(payload *Provision) error {

	currentState := CREATING_PUBLISHED_SERVICE
	nextState := CREATED_PUBLISHED_SERVICE

	err := a.executorUpdateDB(payload, currentState)
	if err != nil {
		return err
	}

	ext_ip, err := CreateKubernetesPublishedService(payload.Metadata.UUID, payload.Spec.Name, payload.Spec)
	if err != nil {
		return err
	}

	if ext_ip != "" {
		payload.Status.ExternalIP = ext_ip
	} else {
		payload.Status.Message = "Deployment successful but external ip not assigned"
	}

	err = a.executorUpdateDB(payload, nextState)
	if err != nil {
		return err
	}

	encoded_payload, err := encodeToBase64(payload)
	if err != nil {
		return err
	}

	err = PushMessage(a.AMQP, encoded_payload)
	if err != nil {
		return err
	}

	return nil
}

func (a *App) success(payload *Provision) error {

	nextState := SUCCESS

	err := a.executorUpdateDB(payload, nextState)
	if err != nil {
		return err
	}

	return nil
}

func (a *App) failed(payload *Provision, err error) error {

	nextState := FAILED
	payload.Status.Message = err.Error()

	err = a.executorUpdateDB(payload, nextState)
	if err != nil {
		return err
	}

	return nil
}

func (a *App) deleted(payload *Provision) error {

	nextState := DELETED

	err := a.executorUpdateDB(payload, nextState)
	if err != nil {
		return err
	}

	return nil
}

func (a *App) deleteDeployment(payload *Provision) error {
	currentState := DELETING_DEPLOYMENT
	nextState := DELETED_DEPLOYMENT

	err := a.executorUpdateDB(payload, currentState)
	if err != nil {
		return err
	}

	err = DeleteKubernetesDeployment(payload.Metadata.UUID, payload.Spec.Name)
	if err != nil {
		return err
	}

	err = a.executorUpdateDB(payload, nextState)
	if err != nil {
		return err
	}

	encoded_payload, err := encodeToBase64(payload)
	if err != nil {
		return err
	}

	err = PushMessage(a.AMQP, encoded_payload)
	if err != nil {
		return err
	}

	return nil
}

func (a *App) deletePublishedService(payload *Provision) error {

	currentState := DELETING_PUBLISHED_SERVICE
	nextState := DELETED_PUBLISHED_SERVICE

	err := a.executorUpdateDB(payload, currentState)
	if err != nil {
		return err
	}

	err = DeleteKubernetesPublishedService(payload.Metadata.UUID, payload.Spec.Name)
	if err != nil {
		return err
	}

	err = a.executorUpdateDB(payload, nextState)
	if err != nil {
		return err
	}

	encoded_payload, err := encodeToBase64(payload)
	if err != nil {
		return err
	}

	err = PushMessage(a.AMQP, encoded_payload)
	if err != nil {
		return err
	}

	return nil
}
