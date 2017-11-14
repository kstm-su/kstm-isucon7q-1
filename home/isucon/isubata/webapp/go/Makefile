GOPATH := ${PWD}
export GOPATH

.PHONY: deploy
deploy: build
	scp ~/isubata/webapp/go/isubata app2:~/isubata/webapp/go/isubata
	ssh root@app2 systemctl restart isubata.golang.service

.PHONY: build
build: commit
	go fmt ./src/isubata/app.go
	go build -v isubata

vet:
	go vet ./src/isubata/...

.PHONY: commit
commit:
	sudo git add ./src/isubata/
	sudo git commit --allow-empty -m 'Build isubata.golang'
