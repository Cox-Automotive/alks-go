package alks

import (
	. "gopkg.in/check.v1"
)

func (s *S) Test_GetMyLoginRole(c *C) {
	testServer.Response(200, nil, getIamLoginRoleResponse)

	oldCreds := s.client.Credentials
	s.client.Credentials = &STS{AccessKey: "abc", SecretKey: "123", SessionToken: "abc123"}

	resp, err := s.client.GetMyLoginRole()

	_ = testServer.WaitRequest()

	c.Assert(resp.LoginRole.Account, Equals, "012345678910/ALKSAdmin")
	c.Assert(resp.LoginRole.IamKeyActive, Equals, true)
	c.Assert(resp.LoginRole.MaxKeyDuration, Equals, 36)
	c.Assert(resp.LoginRole.Role, Equals, "Admin")
	c.Assert(err, IsNil)

	s.client.Credentials = oldCreds
}

func (s *S) Test_GetLoginRole(c *C) {
	testServer.Response(200, nil, getIamLoginRoleResponse)

	resp, err := s.client.GetLoginRole()

	_ = testServer.WaitRequest()

	c.Assert(resp.LoginRole.Account, Equals, "012345678910/ALKSAdmin")
	c.Assert(resp.LoginRole.IamKeyActive, Equals, true)
	c.Assert(resp.LoginRole.MaxKeyDuration, Equals, 36)
	c.Assert(resp.LoginRole.Role, Equals, "Admin")
	c.Assert(err, IsNil)
}
