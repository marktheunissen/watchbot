package jobs

import "io"

type Cmd struct {
	CamIndex int
	Noun     string
	Verb     string
	Obj      string
}

type UploadJob struct {
	CamIndex int
	Caption  string
	Data     io.Reader
}
