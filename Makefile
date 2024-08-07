default: dogenet

.PHONY: clean, test
clean:
	rm -rf ./dogenet

dogenet: clean
	go build -o dogenet ./cmd/dogenet/. 

dev1:
	KEY=$(shell cat dev-key1) IDENT=$(shell cat ident-pub1) go run ./cmd/dogenet --db storage/dev1.db --bind 127.0.0.1:8096 --web 127.0.0.1:8086

dev:
	KEY=$(shell cat dev-key) IDENT=$(shell cat ident-pub) go run ./cmd/dogenet

key:
	go run ./cmd/dogenet genkey

dev-key:
	go run ./cmd/dogenet genkey >dev-key
dev-key1:
	go run ./cmd/dogenet genkey >dev-key1
ident-key:
	go run ./cmd/dogenet genkey >ident-key
	cut -c 65- ident-key >ident-pub
ident-key1:
	go run ./cmd/dogenet genkey >ident-key1
	cut -c 65- ident-key1 >ident-pub1

test:
	go test -v ./test
