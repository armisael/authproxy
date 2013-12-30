package aerrors

type ResponseError struct {
	Status  int
	Message string
	Code    string
}

func (r ResponseError) Error() string {
	return r.Message
}
