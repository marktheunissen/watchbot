
rpi:
	GOARCH=arm GOARM=6 GOOS=linux go build -v upload.go && scp upload pi5:~/bin

run:
	go run -race main.go

.PHONY: rpi run
