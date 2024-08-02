default: dogenet

.PHONY: clean, test
clean:
	rm -rf ./dogenet

dogenet: clean
	go build -o dogenet ./cmd/dogenet/. 

dev1:
	go run ./cmd/dogenet --bind 127.0.0.1:8096 --web 127.0.0.1:8086

dev:
	go run ./cmd/dogenet

test:
	go test -v ./test
