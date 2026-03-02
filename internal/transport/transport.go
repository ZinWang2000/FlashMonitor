package transport

type Sender interface {
	Send(data []byte) error
}

type Transport interface {
	Start() error
	Stop() error
}
