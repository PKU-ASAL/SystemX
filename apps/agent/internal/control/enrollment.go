package control

import "context"

type EnrollmentCommand struct {
	Context       RequestContext
	ManagerURL    string
	Token         string
	UploadHistory bool
}

type UnenrollmentCommand struct {
	Context RequestContext
}

type EnrollmentController interface {
	Enroll(context.Context, EnrollmentCommand) Result
	Unenroll(context.Context, UnenrollmentCommand) Result
}
