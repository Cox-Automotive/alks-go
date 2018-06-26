package alks

import (
	. "github.com/motain/gocheck"
)

func (s *S) Test_CreateIamRole(c *C) {
	testServer.Response(202, nil, iamGetRole)

	resp, err := s.client.CreateIamRole("rolebae", "Admin", false)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.RoleName, Equals, "rolebae")
	c.Assert(resp.RoleType, Equals, "Admin")
}

func (s *S) Test_CreateIamTrustRole(c *C) {
	testServer.Response(202, nil, iamGetRole)

	resp, err := s.client.CreateIamTrustRole("rolebae", "Cross Account", "arn:aws:iam::123456789123:role/test-role")

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.RoleName, Equals, "rolebae")
	c.Assert(resp.RoleType, Equals, "Cross Account")
}

func (s *S) Test_GetIamRole(c *C) {
	testServer.Response(202, nil, iamGetRole)

	resp, err := s.client.GetIamRole("rolebae")

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.RoleName, Equals, "rolebae")
	c.Assert(resp.RoleType, Equals, "Admin")
	c.Assert(resp.Exists, Equals, true)
}

func (s *S) Test_GetIamRoleMissing(c *C) {
	testServer.Response(202, nil, iamGetRole404)

	resp, _ := s.client.GetIamRole("rolebaez")

	_ = testServer.WaitRequest()

	c.Assert(resp, IsNil)
}

func (s *S) Test_DeleteIamRole(c *C) {
	testServer.Response(202, nil, "{}")

	err := s.client.DeleteIamRole("rolebaezzzzz")

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
}

var iamGetRole = `
{
    "roleName": "rolebae",
    "roleType": "Admin",
    "roleArn": "aws:arn:foo",
    "instanceProfileArn": "aws:arn:foo:ip",
    "addedRoleToInstanceProfile": true,
    "errors": [],
    "roleExists": true
}
`

var iamGetRole404 = `
{
    "roleName": "",
    "roleType": "",
    "roleArn": "",
    "instanceProfileArn": "",
    "errors": [],
    "roleExists": false
}
`
