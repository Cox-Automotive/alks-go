package alks

import (
	. "github.com/motain/gocheck"
	"time"
)

func (s *S) Test_CreateSession(c *C) {
	testServer.Response(202, nil, sessionCreate)

	resp, err := s.client.CreateSession(2)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.AccessKey, Equals, "foo")
	c.Assert(resp.SecretKey, Equals, "bar")
	c.Assert(resp.SessionToken, Equals, "baz")
	c.Assert(resp.SessionDuration, Equals, 2)
	c.Assert(resp.Expires.After(time.Now()), Equals, true)
}

func (s *S) Test_CreateSessionBadTime(c *C) {
	resp, err := s.client.CreateSession(1)

	c.Assert(err, NotNil)
	c.Assert(resp, IsNil)
}

var sessionCreate = `
{
    "accessKey": "foo",
    "secretKey": "bar",
    "sessionToken": "baz"
}
`
