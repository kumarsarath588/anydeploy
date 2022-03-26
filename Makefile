setup_deps:
	go mod tidy
	go mod vendor

tests: setup_deps
	go test -v

build: tests setup_deps
	CGO_ENABLED=0 go build -o ./anydeploy main.go

run: setup_deps
	go run main.go

setup_docker_centos:
	sudo yum install -y --quiet yum-utils
	sudo yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
	sudo yum install -y --quiet docker-ce docker-compose

docker_image_build: setup_docker_centos
	docker build --no-cache -t anydeploy:1.0 .

docker_compose_up: setup_docker_centos
	docker-compose up -d

docker_compose_down:
	docker-compose down --volumes

docker_cleanup: docker_compose_down
	docker system prune -a -f

helm_download_deps:
	helm dep update ./helm-chart/anydeploy

helm_install: helm_download_deps
	helm upgrade --install anydeploy ./helm-chart/anydeploy

helm_test:
	helm test anydeploy

helm_install_test: helm_install helm_test

helm_delete:
	helm delete anydeploy

all: setup_deps build docker_compose_up