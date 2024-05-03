default: dogenet

.PHONY: clean, test
clean:
	rm -rf ./dogenet

dogenet: clean
	go build -o dogenet ./cmd/dogenet/. 

dev:
	go run ./cmd/dogenet 127.0.0.1

test:
	go test -v ./test
