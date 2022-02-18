package alks

import (
	. "github.com/motain/gocheck"
)

func (s *S) Test_CreateIamRole(c *C) {
	testServer.Response(202, nil, iamGetRole)

	resp, err := s.client.CreateIamRole("rolebae", "Admin", nil, false, false)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.RoleName, Equals, "rolebae")
	c.Assert(resp.RoleType, Equals, "Admin")
	c.Assert(resp.MaxSessionDurationInSeconds, Equals, 3600)
}

func (s *S) Test_CreateIamRoleTemplateFields(c *C) {
	testServer.Response(202, nil, iamGetRoleTemplateFields)

	templateFields := map[string]string{
		"A": "B",
		"C": "D",
	}
	resp, err := s.client.CreateIamRole("rolebae", "Admin", templateFields, false, false)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.RoleName, Equals, "rolebae")
	c.Assert(resp.RoleType, Equals, "Admin")
	c.Assert(resp.TemplateFields["A"], Equals, templateFields["A"])
	c.Assert(resp.TemplateFields["C"], Equals, templateFields["C"])
	c.Assert(resp.MaxSessionDurationInSeconds, Equals, 3600)
}

func (s *S) Test_CreateIamRoleOptions(c *C) {
	testServer.Response(202, nil, iamGetRoleOptions)

	templateFields := map[string]string{
		"A": "B",
		"C": "D",
	}

	options := CreateIamRoleOptions{0, false, templateFields, 7200}

	resp, err := s.client.CreateIamRoleWithOptions("rolebae", "Admin", options)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.RoleName, Equals, "rolebae")
	c.Assert(resp.RoleType, Equals, "Admin")
	c.Assert(resp.TemplateFields["A"], Equals, templateFields["A"])
	c.Assert(resp.TemplateFields["C"], Equals, templateFields["C"])
	c.Assert(resp.MaxSessionDurationInSeconds, Equals, 7200)
}

func (s *S) Test_CreateIamTrustRole(c *C) {
	testServer.Response(202, nil, iamGetTrustRole)

	resp, err := s.client.CreateIamTrustRole("test-cross-role", "Cross Account", "arn:aws:iam::123456789123:role/test-role", false)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.RoleName, Equals, "test-cross-role")
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
	c.Assert(resp.AlksAccess, NotNil)
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

func (s *S) Test_AddRoleMachineIdentity(c *C) {
	testServer.Response(202, nil, machineIdentityResponse)

	resp, err := s.client.AddRoleMachineIdentity("arn:aws:iam::123456789123:role/test-role")

	_ = testServer.WaitRequest()

	c.Assert(resp.MachineIdentityArn, Equals, "arn:aws:iam::123456789123:role/acct-managed/test123")
	c.Assert(err, IsNil)
}

func (s *S) Test_DeleteRoleMachineIdentity(c *C) {
	testServer.Response(202, nil, machineIdentityResponse)

	resp, err := s.client.DeleteRoleMachineIdentity("arn:aws:iam::123456789123:role/test-role")

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp.MachineIdentityArn, Equals, "arn:aws:iam::123456789123:role/acct-managed/test123")
}

func (s *S) Test_SearchRoleMachineIdentity(c *C) {
	testServer.Response(202, nil, machineIdentityResponse)

	resp, err := s.client.SearchRoleMachineIdentity("arn:aws:iam::123456789123:/role/test-role")

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp.MachineIdentityArn, Equals, "arn:aws:iam::123456789123:role/acct-managed/test123")
}

var iamGetRole = `
{
    "roleName": "rolebae",
    "roleType": "Admin",
    "roleArn": "aws:arn:foo",
    "instanceProfileArn": "aws:arn:foo:ip",
    "addedRoleToInstanceProfile": true,
    "errors": [],
		"roleExists": true,
		"machineIdentity": false,
		"maxSessionDurationInSeconds":3600
}
`

var iamGetRoleTemplateFields = `
{
    "roleName": "rolebae",
    "roleType": "Admin",
    "roleArn": "aws:arn:foo",
    "instanceProfileArn": "aws:arn:foo:ip",
    "addedRoleToInstanceProfile": true,
    "errors": [],
		"roleExists": true,
		"machineIdentity": false,
		"templateFields": {
			"A": "B",
			"C": "D"
		},
		"maxSessionDurationInSeconds": 3600
}
`

var iamGetRoleOptions = `
{
    "roleName": "rolebae",
    "roleType": "Admin",
    "roleArn": "aws:arn:foo",
    "instanceProfileArn": "aws:arn:foo:ip",
    "addedRoleToInstanceProfile": true,
    "errors": [],
		"roleExists": true,
		"machineIdentity": false,
		"templateFields": {
			"A": "B",
			"C": "D"
		},
		"maxSessionDurationInSeconds": 7200
}
`

var iamGetTrustRole = `
{
    "roleName": "test-cross-role",
    "roleType": "Cross Account",
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

var machineIdentityResponse = `
{
	"machineIdentityArn": "arn:aws:iam::123456789123:role/acct-managed/test123"
}
`
