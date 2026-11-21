package flows

type flowError string

func (e flowError) Error() string { return string(e) }
