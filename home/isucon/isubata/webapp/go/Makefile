GOPATH := ${PWD}
export GOPATH

build: @commit
	go fmt ./src/isubata/app.go
	go build -v isubata

vet:
	go vet ./src/isubata/...

.PHONY: @commit
@commit:
	sudo git add ./src/isubata/
	sudo git commit -m 'Build isubata.golang'
