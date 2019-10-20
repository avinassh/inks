
all: inks

inks: go.mod *.go
	go build -o inks

clean:
	rm -f inks
