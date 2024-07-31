default: dogenet

.PHONY: clean, test
clean:
	rm -rf ./dogenet

dogenet: clean
	go build -o dogenet ./cmd/dogenet/. 

dev1:
	go run ./cmd/dogenet 127.0.0.1 1 1111111111222222222233333333334411111111112222222222333333333344 127.0.0.1:8095

dev:
	go run ./cmd/dogenet 127.0.0.1

test:
	go test -v ./test
