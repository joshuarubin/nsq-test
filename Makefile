all: run stop clean

nsq-test:
	go build

nsqlookupd:
	nsqlookupd > /dev/null 2>&1 &

nsqd: nsqlookupd
	nsqd --lookupd-tcp-address=127.0.0.1:4160 > /dev/null 2>&1 &

nsqadmin: nsqlookupd
	nsqadmin --lookupd-http-address=127.0.0.1:4161 > /dev/null 2>&1 &

nsq_to_file: nsqlookupd
	nsq_to_file --topic=test --output-dir=/tmp --lookupd-http-address=127.0.0.1:4161 > /dev/null 2>&1 &

run: nsqd
	go run ./nsq-test.go

stop:
	-killall nsqlookupd nsqd nsqadmin nsq_to_file

clean:
	-rm *.dat
