
all: inks

inks: *.go
	go build -o inks

clean:
	rm -f inks
