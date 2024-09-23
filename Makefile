default: dogenet

.PHONY: clean, test
clean:
	rm -rf ./dogenet

dogenet: clean
	go build -o dogenet ./cmd/dogenet/. 

dev:
	KEY=$(shell cat dev-key) IDENT=$(shell cat ident-pub) go run ./cmd/dogenet --local --public 127.0.0.1

dev1:
	KEY=$(shell cat dev-key1) IDENT=$(shell cat ident-pub1) go run ./cmd/dogenet --db storage/dev1.db --local --public 127.0.0.1:8096 --bind 127.0.0.1:8096 --web 127.0.0.1:8086

dev2:
	KEY=$(shell cat dev-key2) IDENT=$(shell cat ident-pub2) go run ./cmd/dogenet --db storage/dev2.db --local --public 127.0.0.1:8097 --bind 127.0.0.1:8097 --web 127.0.0.1:8087

dev3:
	KEY=$(shell cat dev-key3) IDENT=$(shell cat ident-pub3) go run ./cmd/dogenet --db storage/dev3.db --local --public 127.0.0.1:8098 --bind 127.0.0.1:8098 --web 127.0.0.1:8088

key:
	go run ./cmd/dogenet genkey

dev-key:
	go run ./cmd/dogenet genkey dev-key
ident-key:
	go run ./cmd/dogenet genkey ident-key ident-pub
dev-key1:
	go run ./cmd/dogenet genkey dev-key1
	go run ./cmd/dogenet genkey dev-key2
	go run ./cmd/dogenet genkey dev-key3
ident-key1:
	go run ./cmd/dogenet genkey ident-key1 ident-pub1
	go run ./cmd/dogenet genkey ident-key2 ident-pub2
	go run ./cmd/dogenet genkey ident-key3 ident-pub3

test:
	go test -v ./test
