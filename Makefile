default: dogenet

.PHONY: clean, test
clean:
	rm -rf ./dogenet

dogenet: clean
	go build -o dogenet ./cmd/dogenet/. 


dev:
	go run ./cmd/dogenet 


test:
	go test -v ./test
