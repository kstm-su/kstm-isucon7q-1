GOPATH := ${PWD}
export GOPATH

.PHONY: deploy
deploy: build
	sudo systemctl start isubata.golang.service
	ssh root@app2 systemctl stop isubata.golang.service
	scp ~/isubata/webapp/go/isubata app2:~/isubata/webapp/go/isubata
	ssh root@app2 systemctl start isubata.golang.service

.PHONY: commit
commit: deploy
	sudo git add ./src/isubata/
	sudo git commit --allow-empty -m 'Build isubata.golang'

.PHONY: build
build:
	go fmt ./src/isubata/*.go
	sudo systemctl stop isubata.golang.service
	go build -v isubata

.PHONY: test
test:
	go fmt ./src/isubata/*.go
	go test -v isubata

vet:
	go vet ./src/isubata/...
