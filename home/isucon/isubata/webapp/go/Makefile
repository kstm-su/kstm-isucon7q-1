GOPATH := ${PWD}
export GOPATH

.PHONY: commit
commit: deploy
	sudo git add ./src/isubata/
	sudo git commit --allow-empty -m 'Build isubata.golang'

.PHONY: deploy
deploy: build
	sudo systemctl start isubata.golang.service
	ssh root@app2 systemctl stop isubata.golang.service
	scp ~/isubata/webapp/go/isubata app2:~/isubata/webapp/go/isubata
	ssh root@app2 systemctl start isubata.golang.service

.PHONY: build
build:
	go fmt ./src/isubata/app.go
	sudo systemctl stop isubata.golang.service
	go build -v isubata

vet:
	go vet ./src/isubata/...
