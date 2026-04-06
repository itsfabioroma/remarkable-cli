build:
	go build -o remarkable .

install:
	go install .

test:
	go test ./...

clean:
	rm -f remarkable
