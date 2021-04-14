package alks

import (
	. "github.com/motain/gocheck"
)

func (s *S) Test_IsIamEnabledMI(c *C) {
	testServer.Response(202, nil, iamEnabledMI)

	resp, err := s.client.IsIamEnabled("arn:aws:iam::accountNo:role/acct-managed/test-mi")
	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.RoleArn, Equals, "arn:aws:iam::accountNo:role/acct-managed/test-mi")
	c.Assert(resp.IamEnabled, Equals, true)
}

func (s *S) Test_IsIamEnabledSTS(c *C) {
	testServer.Response(202, nil, iamEnabledSTS)

	resp, err := s.client.IsIamEnabled("")
	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.RoleArn, Equals, "arn:aws:sts::accountNo:assumed-role/Admin/userid")
	c.Assert(resp.IamEnabled, Equals, true)
}

var iamEnabledMI = `
{
    "roleArn": "arn:aws:iam::accountNo:role/acct-managed/test-mi",
    "iamEnabled": true
}
`

var iamEnabledSTS = `
{
	"roleArn": "arn:aws:sts::accountNo:assumed-role/Admin/userid",
	"iamEnabled": true
}
`
