package alks

import (
	"encoding/json"

	. "github.com/motain/gocheck"
)

func (s *S) Test_CreateIamRole(c *C) {
	testServer.Response(202, nil, iamCreateRole)

	roleName := "rolebae"
	roleType := "Amazon EC2"
	opts := &CreateIamRoleOptions{
		RoleName: &roleName,
		RoleType: &roleType,
	}

	resp, err := s.client.CreateIamRole(opts)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.RoleName, Equals, "rolebae")
	c.Assert(resp.RoleType, Equals, "Amazon EC2")
	c.Assert(resp.MaxSessionDurationInSeconds, Equals, 3600)
}

func (s *S) Test_CreateIamRoleBad(c *C) {
	testServer.Response(400, nil, iamCreateRole400)

	roleName := "rolebae"
	roleType := "Amazon EC2"
	opts := &CreateIamRoleOptions{
		RoleName: &roleName,
		RoleType: &roleType,
	}

	resp, err := s.client.CreateIamRole(opts)

	_ = testServer.WaitRequest()

	c.Assert(err, NotNil)
	c.Assert(resp, IsNil)
}

func (s *S) Test_CreateIamRoleTemplateFields(c *C) {
	testServer.Response(202, nil, iamGetRoleTemplateFields)

	roleName := "rolebae"
	roleType := "Amazon EC2"
	templateFields := map[string]string{
		"A": "B",
		"C": "D",
	}
	opts := &CreateIamRoleOptions{
		RoleName:       &roleName,
		RoleType:       &roleType,
		TemplateFields: &templateFields,
	}

	resp, err := s.client.CreateIamRole(opts)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.RoleName, Equals, "rolebae")
	c.Assert(resp.RoleType, Equals, "Amazon EC2")
	c.Assert(resp.TemplateFields["A"], Equals, templateFields["A"])
	c.Assert(resp.TemplateFields["C"], Equals, templateFields["C"])
	c.Assert(resp.MaxSessionDurationInSeconds, Equals, 3600)
}

func (s *S) Test_CreateIamRoleOptions(c *C) {
	testServer.Response(202, nil, iamGetRoleOptions)

	roleName := "rolebae"
	roleType := "Amazon EC2"
	templateFields := map[string]string{
		"A": "B",
		"C": "D",
	}
	maxSessionDuration := 7200
	opts := &CreateIamRoleOptions{
		RoleName:                    &roleName,
		RoleType:                    &roleType,
		TemplateFields:              &templateFields,
		MaxSessionDurationInSeconds: &maxSessionDuration,
	}

	resp, err := s.client.CreateIamRole(opts)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.RoleName, Equals, "rolebae")
	c.Assert(resp.RoleType, Equals, "Amazon EC2")
	c.Assert(resp.TemplateFields["A"], Equals, templateFields["A"])
	c.Assert(resp.TemplateFields["C"], Equals, templateFields["C"])
	c.Assert(resp.MaxSessionDurationInSeconds, Equals, 7200)
}

func (s *S) Test_CreateIamRoleWithTrustPolicy(c *C) {
	testServer.Response(202, nil, iamGetRoleTrustPolicy)

	roleName := "rolebae"
	trustPolicy := new(map[string]interface{})
	byt := []byte(`{"Version":"2012-10-17","Statement":[{"Sid":"","Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	json.Unmarshal(byt, trustPolicy)

	templateFields := map[string]string{
		"A": "B",
		"C": "D",
	}
	maxSessionDuration := 7200
	opts := &CreateIamRoleOptions{
		RoleName:                    &roleName,
		TrustPolicy:                 trustPolicy,
		TemplateFields:              &templateFields,
		MaxSessionDurationInSeconds: &maxSessionDuration,
	}

	resp, err := s.client.CreateIamRole(opts)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.RoleName, Equals, "rolebae")
	c.Assert(resp.TrustPolicy, DeepEquals, *trustPolicy)
	c.Assert(resp.TemplateFields["A"], Equals, templateFields["A"])
	c.Assert(resp.TemplateFields["C"], Equals, templateFields["C"])
	c.Assert(resp.MaxSessionDurationInSeconds, Equals, 7200)
}

func (s *S) Test_CreateIamRoleWithTrustPolicyandRoleType(c *C) {
	testServer.Response(202, nil, iamGetRoleOptions)

	roleName := "rolebae"
	roleType := "Amazon EC2"
	trustPolicy := new(map[string]interface{})
	byt := []byte(`{"Version":"2012-10-17","Statement":[{"Sid":"","Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	json.Unmarshal(byt, trustPolicy)
	templateFields := map[string]string{
		"A": "B",
		"C": "D",
	}
	maxSessionDuration := 7200
	opts := &CreateIamRoleOptions{
		RoleName:                    &roleName,
		RoleType:                    &roleType,
		TrustPolicy:                 trustPolicy,
		TemplateFields:              &templateFields,
		MaxSessionDurationInSeconds: &maxSessionDuration,
	}

	resp, err := s.client.CreateIamRole(opts)

	c.Assert(err, NotNil)
	c.Assert(resp, IsNil)
}

func (s *S) Test_CreateIamRoleWithTags(c *C) {
	testServer.Response(202, nil, iamGetRoleOptions)

	roleName := "rolebae"
	roleType := "Amazon EC2"
	tags := []Tag{
		{
			Key:   "cai-owner",
			Value: "123456",
		},
		{
			Key:   "cai-person",
			Value: "161803",
		},
	}

	opts := &CreateIamRoleOptions{
		RoleName: &roleName,
		RoleType: &roleType,
		Tags:     &tags,
	}

	resp, err := s.client.CreateIamRole(opts)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.RoleName, Equals, "rolebae")
	c.Assert(resp.RoleType, Equals, "Amazon EC2")
}

func (s *S) Test_CreateIamTrustRole(c *C) {
	testServer.Response(202, nil, iamGetTrustRole)

	roleName := "test-cross-role"
	roleType := "Cross Account"
	roleTrust := "arn:aws:iam::123456789123:role/test-role"
	templateFields := map[string]string{
		"A": "B",
		"C": "D",
	}
	opts := &CreateIamRoleOptions{
		RoleName:       &roleName,
		RoleType:       &roleType,
		TrustArn:       &roleTrust,
		TemplateFields: &templateFields,
	}

	resp, err := s.client.CreateIamTrustRole(opts)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.RoleName, Equals, "test-cross-role")
	c.Assert(resp.RoleType, Equals, "Cross Account")
}

func (s *S) Test_GetIamRole(c *C) {
	testServer.Response(202, nil, iamGetRole)

	trustPolicy := new(map[string]interface{})
	byt := []byte(`{"Version":"2012-10-17","Statement":[{"Sid":"","Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)
	json.Unmarshal(byt, trustPolicy)
	resp, err := s.client.GetIamRole("rolebae")

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.RoleName, Equals, "rolebae")
	c.Assert(resp.TrustPolicy, DeepEquals, *trustPolicy)
	c.Assert(resp.Exists, Equals, true)
	c.Assert(resp.AlksAccess, NotNil)
	c.Assert(len(resp.Tags), Equals, 2)
	c.Assert(resp.Tags[0].Key, Equals, "foo")
	c.Assert(resp.Tags[0].Value, Equals, "bar")
	c.Assert(resp.Tags[1].Key, Equals, "cloud")
	c.Assert(resp.Tags[1].Value, Equals, "railway")
	c.Assert(resp.MaxSessionDurationInSeconds, Equals, 3600)
}

func (s *S) Test_GetIamRoleMissing(c *C) {
	testServer.Response(404, nil, iamGetRole404)

	resp, err := s.client.GetIamRole("rolebaez")

	_ = testServer.WaitRequest()

	c.Assert(resp, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.StatusCode, Equals, 404)
}

func (s *S) Test_GetIamRoleInternalError(c *C) {
	testServer.Response(500, nil, iamGetRole500)

	resp, err := s.client.GetIamRole("rolebaez")

	_ = testServer.WaitRequest()

	c.Assert(resp, IsNil)
	c.Assert(err, NotNil)
	c.Assert(err.StatusCode, Equals, 500)
}

func (s *S) Test_UpdateIamRoleTags(c *C) {
	testServer.Response(202, nil, updateRoleResponse)

	roleName := "test-update-role"
	tags := []Tag{
		{
			Key:   "cai-owner",
			Value: "123456",
		},
		{
			Key:   "cai-person",
			Value: "161803",
		},
	}
	req := &UpdateIamRoleRequest{RoleName: &roleName, Tags: &tags}
	resp, err := s.client.UpdateIamRole(req)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(*resp.RoleName, Equals, roleName)
	c.Assert(*resp.Tags, NotNil)
}

func (s *S) Test_UpdateIamRoleTrustPolicy(c *C) {
	testServer.Response(202, nil, updateRoleResponse)

	roleName := "test-update-role"
	trustPolicy := new(map[string]interface{})
	json.Unmarshal(
		[]byte(`{"Version":"2012-10-17","Statement":[{"Sid":"","Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}`),
		trustPolicy)
	req := &UpdateIamRoleRequest{RoleName: &roleName, TrustPolicy: trustPolicy}
	resp, err := s.client.UpdateIamRole(req)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(*resp.RoleName, Equals, roleName)
	c.Assert(*resp.Tags, NotNil)
}

func (s *S) Test_UpdateIamRoleTagsAndTrustPolicy(c *C) {
	testServer.Response(202, nil, updateRoleResponse)

	roleName := "test-update-role"
	tags := []Tag{
		{
			Key:   "cai-owner",
			Value: "123456",
		},
		{
			Key:   "cai-person",
			Value: "161803",
		},
	}
	trustPolicy := new(map[string]interface{})
	json.Unmarshal(
		[]byte(`{"Version":"2012-10-17","Statement":[{"Sid":"","Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}`),
		trustPolicy)
	req := &UpdateIamRoleRequest{RoleName: &roleName, Tags: &tags, TrustPolicy: trustPolicy}
	resp, err := s.client.UpdateIamRole(req)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(*resp.RoleName, Equals, roleName)
	c.Assert(*resp.Tags, NotNil)
}

func (s *S) Test_DeleteIamRole(c *C) {
	testServer.Response(202, nil, "{}")

	err := s.client.DeleteIamRole("rolebaezzzzz")

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
}

func (s *S) Test_DeleteIamRoleNotFound(c *C) {
	testServer.Response(404, nil, iamDeleteRole404)

	err := s.client.DeleteIamRole("rolebaezzzzz")

	_ = testServer.WaitRequest()

	c.Assert(err, NotNil)
}

func (s *S) Test_DeleteIamRoleNotFoundNoReqId(c *C) {
	testServer.Response(404, nil, iamDeleteRole404NoReqId)

	err := s.client.DeleteIamRole("rolebaezzzzz")

	_ = testServer.WaitRequest()

	c.Assert(err, NotNil)
}

func (s *S) Test_DeleteIamRoleInternalError(c *C) {
	testServer.Response(500, nil, iamDeleteRole500)

	err := s.client.DeleteIamRole("rolebaezzzzz")

	_ = testServer.WaitRequest()

	c.Assert(err, NotNil)
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

var iamCreateRole = `
{
    "roleName": "rolebae",
    "roleType": "Amazon EC2",
	"trustPolicy": {"Version":"2012-10-17","Statement":[{"Sid":"","Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]},
    "roleArn": "aws:arn:foo",
    "instanceProfileArn": "aws:arn:foo:ip",
    "addedRoleToInstanceProfile": true,
    "errors": [],
    "roleExists": true,
    "machineIdentity": false,
    "maxSessionDurationInSeconds":3600,
    "tags": [
        {
            "key": "foo",
            "value": "bar"
        },
        {
            "key": "cloud",
            "value": "railway"
        }
    ]
}
`

var iamGetRole = `
{
    "roleName": "rolebae",
	"trustPolicy": {"Version":"2012-10-17","Statement":[{"Sid":"","Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]},
    "roleArn": "aws:arn:foo",
    "instanceProfileArn": "aws:arn:foo:ip",
    "addedRoleToInstanceProfile": true,
    "errors": [],
    "roleExists": true,
    "machineIdentity": false,
    "maxSessionDurationInSeconds":3600,
    "tags": [
        {
            "key": "foo",
            "value": "bar"
        },
        {
            "key": "cloud",
            "value": "railway"
        }
    ]
}
`

var iamGetRoleTemplateFields = `
{
    "roleName": "rolebae",
    "roleType": "Amazon EC2",
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

var iamGetRoleTrustPolicy = `
{
    "roleName": "rolebae",
    "roleType": "Amazon EC2",
	"trustPolicy": {"Version":"2012-10-17","Statement":[{"Sid":"","Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]},
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

var iamGetRoleOptions = `
{
    "roleName": "rolebae",
    "roleType": "Amazon EC2",
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

var iamGetRole500 = `
{
    "roleName": "",
    "roleType": "",
    "roleArn": "",
    "instanceProfileArn": "",
    "errors": ["Internal Server Error"],
    "roleExists": false
}
`

var iamCreateRole400 = `
{
    "statusMessage": "Role already exists with the same name: TestGo",
    "errors": [
        "Role already exists with the same name: TestGo"
    ],
    "requestId": "mqtwkzij",
    "role": "",
    "account": "",
    "action": "CREATE",
    "addedRoleToInstanceProfile": false,
    "roleArn": "NA",
    "denyArns": "NA",
    "roleName": "",
    "includeDefaultPolicy": 1,
    "maxSessionDurationInSeconds": 3600
}`

var iamDeleteRole404 = `
{
    "statusMessage": "Failed",
    "errors": [
        "Role not found"
    ],
    "requestId": "",
    "role": "",
    "account": "",
    "action": "DELETE",
    "roleName": "TestGo"
}`

var iamDeleteRole404NoReqId = `
{
    "statusMessage": "Failed",
    "errors": [
        "Role not found"
    ],
    "requestId": "",
    "role": "",
    "account": "",
    "action": "DELETE",
    "roleName": ""
}`

var iamDeleteRole500 = `
{
    "statusMessage": "Failed",
    "errors": [
        "Internal Server Error"
    ],
    "requestId": "",
    "role": "",
    "account": "",
    "action": "DELETE",
    "roleName": ""
}`

var machineIdentityResponse = `
{
    "machineIdentityArn": "arn:aws:iam::123456789123:role/acct-managed/test123"
}
`

var updateRoleResponse = `
{
	"roleArn": "aws:arn:foo",
	"roleName": "test-update-role",
	"basicAuthUsed": false,
	"roleExists": true,
	"instanceProfileArn": "aws:arn:foo:ip",
	"isMachineIdentity": true,
	"tags": [{"key":"cai-owner","value":"123456"},{"key":"cai-person","value":"161803"}],
	"errors": []
}
`
