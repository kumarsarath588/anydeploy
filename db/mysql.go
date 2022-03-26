package db

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
)

const (
	queryGetByID  string = "SELECT uuid, name, type, state, payload FROM deployments where uuid = '%s';"
	executeInsert string = "INSERT INTO deployments (uuid, name, type, state, payload) VALUES (?, ?, ?, ?, ?);"
	executeUpdate string = "UPDATE deployments SET name = ?, type = ?, state = ?, payload =? WHERE uuid = ?;"
)

type DBPayload struct {
	UUID   string
	Name   string
	Type   string
	State  string
	Base64 string
}

func assertRowsAffected(result sql.Result, expectedNumRows int64) error {
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	} else if rowsAffected != expectedNumRows {
		return fmt.Errorf("unexpected number of DB Records affected: %d", (rowsAffected))
	}

	return nil
}

func InsertEntry(db *sql.DB, payload *DBPayload) error {

	prepStmt, err := db.Prepare(executeInsert)
	if err != nil {
		return err
	}
	defer prepStmt.Close()
	result, err := prepStmt.Exec(payload.UUID, payload.Name, payload.Type, payload.State, payload.Base64)

	if err != nil {
		return err
	}

	err = assertRowsAffected(result, 1)
	if err != nil {
		return err
	}

	log.Printf("Successfully inserted provisioning data with uuid: %s", payload.UUID)

	return nil
}

func GetEntry(db *sql.DB, uuid string) (*DBPayload, error) {

	rows, err := db.Query(fmt.Sprintf(queryGetByID, uuid))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	exists := rows.Next()
	if !exists {
		err = rows.Err()
		if err != nil {
			return nil, err
		}

		return nil, fmt.Errorf("no entry found with uuid '%s'", uuid)

	}

	var UUID, name, provision_type, state, base64 string

	if err := rows.Scan(&UUID, &name, &provision_type, &state, &base64); err != nil {
		return nil, err
	}

	return &DBPayload{
		UUID:   UUID,
		Name:   name,
		Type:   provision_type,
		State:  state,
		Base64: base64,
	}, nil
}

func UpdateEntry(db *sql.DB, payload *DBPayload) error {

	prepStmt, err := db.Prepare(executeUpdate)
	if err != nil {
		return err
	}
	defer prepStmt.Close()

	result, err := prepStmt.Exec(payload.Name, payload.Type, payload.State, payload.Base64, payload.UUID)

	if err != nil {
		return err
	}

	err = assertRowsAffected(result, 1)
	if err != nil {
		return err
	}

	log.Printf("Updated provisioning task uuid '%s' to '%s'.", payload.UUID, payload.State)

	return nil
}
