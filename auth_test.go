package main

import (
	"testing"
)

func TestCacheJWT(t *testing.T) {
	// if !(os.Getenv("AWS_COGNITO_USER_POOL_ID") != "" && os.Getenv("AWS_COGNITO_REGION") != "") {
	// 	t.Skip("requires AWS Cognito environment variables")
	// }

	auth := NewAuth(&Config{
		CognitoRegion:     "ap-south-1",
		CognitoUserPoolID: "ap-south-1_Ve673ueYP",
	})

	err := auth.CacheJWK()
	if err != nil {
		t.Error(err)
	}

	jwt := "eyJraWQiOiJcL1FYWUs3MkdCdU9TazNUVE93ZjNWT09NUUhvbk9ET2hFaGhBakdKZDdjST0iLCJhbGciOiJSUzI1NiJ9.eyJzdWIiOiIyMzdhNjk1Zi0wZmZjLTQxYmItOGViYi0yYzM4Yzk5MjczMWUiLCJlbWFpbF92ZXJpZmllZCI6dHJ1ZSwiaXNzIjoiaHR0cHM6XC9cL2NvZ25pdG8taWRwLmFwLXNvdXRoLTEuYW1hem9uYXdzLmNvbVwvYXAtc291dGgtMV9WZTY3M3VlWVAiLCJjb2duaXRvOnVzZXJuYW1lIjoiMjM3YTY5NWYtMGZmYy00MWJiLThlYmItMmMzOGM5OTI3MzFlIiwib3JpZ2luX2p0aSI6ImRmZTIxYzgwLTQ2NjEtNDhiZi04YmM0LTUwM2FjOWFhMzkzMSIsImF1ZCI6IjRkZHJlOWhzYWlmbmk5ZGVuNTExZzMycGxlIiwiZXZlbnRfaWQiOiI2YzViMzY2Yy0wZTBmLTQ0OGUtYmUxYi0zM2FkZGQ5ODdmNGEiLCJ0b2tlbl91c2UiOiJpZCIsImF1dGhfdGltZSI6MTY2NTk5ODMwOSwiZXhwIjoxNjY2MDAxOTA5LCJpYXQiOjE2NjU5OTgzMDksImp0aSI6ImNiM2U2NjA2LWEwODItNDdlZS1hMjc0LTNlMmY4ZTQ4NmVlYyIsImVtYWlsIjoiZGlsYW5rYS5nYW1hZ2VAa2xhay5jb20ifQ.Mu4cxq0Evt-211SiqFfiV7oMERHHUIFZ5ZFMpv2rHvDVuEoA75BTZaljbTvS99liiURSTFJnIdL8KZUi2xbUzeTg70MIK6E09GoB_Q3YbVMmg4QioVE4Ki9CFwenWb1ud4l2eMHZWPiHhaAXmykrOv14AkGXeYkm8qoKyCoNY2OXcPfrfaaCWF7BPIzkCPe3QZ1q15AnBXDCXCtJq-ZzJPyrfrXy0ZM_SBHS22QIj27We58EBHGxNYmdvnUZB-MvbeAuSEqoGvereT74lEy9C9G1H-jheY617HPgUPyJnTmxwu5WmbC6KX-KywYuuQz4AUL-e_l7TJ6VBjvK5Lo0lg"

	token, err := auth.ParseJWT(jwt)
	// sub, err := auth.sub(token)
	// if err == nil {
	// 	t.Error(fmt.Errorf("Found sub %s", sub))
	// }

	if err != nil {
		t.Error(err)
	}

	if !token.Valid {
		t.Fail()
	}
}
