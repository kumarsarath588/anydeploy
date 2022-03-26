package main

import (
	clientApiv1 "anydeploy/client"
	serverApiv1 "anydeploy/server"
	"flag"
	"log"
	"os"
)

func main() {
	serverMode := flag.Bool("server", false, "Run as server")
	flag.Parse()

	if *serverMode {
		log.Printf("Running process in server mode")
		a := serverApiv1.App{}
		a.Initialize(
			os.Getenv("APP_DB_USERNAME"),
			os.Getenv("APP_DB_PASSWORD"),
			os.Getenv("APP_DB_HOST"),
			os.Getenv("APP_DB_PORT"),
			os.Getenv("APP_DB_NAME"),
			os.Getenv("AMQP_CONNECTION"),
			os.Getenv("AMQP_QUEUE_NAME"),
		)
		a.Run(":8080")

	} else {
		log.Printf("Running process in worker mode")
		a := clientApiv1.App{}
		a.Initialize(
			os.Getenv("APP_DB_USERNAME"),
			os.Getenv("APP_DB_PASSWORD"),
			os.Getenv("APP_DB_HOST"),
			os.Getenv("APP_DB_PORT"),
			os.Getenv("APP_DB_NAME"),
			os.Getenv("AMQP_CONNECTION"),
			os.Getenv("AMQP_QUEUE_NAME"),
		)
		a.Run()

	}
}
