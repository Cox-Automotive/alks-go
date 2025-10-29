package alks

import (
	"time"

	. "gopkg.in/check.v1"
)

func (s *S) Test_CreateSession(c *C) {
	testServer.Response(200, nil, getNonIamLoginRoleResponse)
	testServer.Response(202, nil, sessionCreate)

	resp, err := s.client.CreateSession(2, false)

	_ = testServer.WaitRequest()
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
	testServer.Response(200, nil, getNonIamLoginRoleResponse)

	resp, err := s.client.CreateSession(42, false)

	_ = testServer.WaitRequest()

	c.Assert(err, NotNil)
	c.Assert(resp, IsNil)
}

func (s *S) Test_CreateSessionIam(c *C) {
	testServer.Response(200, nil, getNonIamLoginRoleResponse)
	testServer.Response(202, nil, sessionCreate)

	resp, err := s.client.CreateSession(1, true)

	_ = testServer.WaitRequest()
	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.AccessKey, Equals, "foo")
	c.Assert(resp.SecretKey, Equals, "bar")
	c.Assert(resp.SessionToken, Equals, "baz")
	c.Assert(resp.SessionDuration, Equals, 1)
	c.Assert(resp.Expires.After(time.Now()), Equals, true)
}

func getIndexByAccount(accounts []AccountRole, account string) (index int) {
	for i, v := range accounts {
		if v.Account == account {
			return i
		}
	}

	return -1
}

func (s *S) Test_GetAccountsPowerUser(c *C) {
	testServer.Response(202, nil, getAccounts)

	resp, err := s.client.GetAccounts()

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	var index = getIndexByAccount(resp.Accounts, "123456/ALKSPowerUser - foobarbaz")
	c.Assert(resp.Accounts[index].Account, Equals, "123456/ALKSPowerUser - foobarbaz") // make sure account name is transformed to key
	c.Assert(resp.Accounts[index].Role, Equals, "PowerUser")
	c.Assert(resp.Accounts[index].IamActive, Equals, false)
	c.Assert(resp.Accounts[index].SkypieaAccount, Equals, SkypieaAccount{Account: "0123456789", Alias: "awsalks", Label: "ALKS - Prod"})
}

func (s *S) Test_GetAccountsIAMAdmin(c *C) {
	testServer.Response(202, nil, getAccounts)

	resp, err := s.client.GetAccounts()

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	var index = getIndexByAccount(resp.Accounts, "234567/ALKSIAMAdmin - foobarbaz2")
	c.Assert(resp.Accounts[index].Account, Equals, "234567/ALKSIAMAdmin - foobarbaz2") // make sure account name is transformed to key
	c.Assert(resp.Accounts[index].Role, Equals, "IAMAdmin")
	c.Assert(resp.Accounts[index].IamActive, Equals, true)
	c.Assert(resp.Accounts[index].SkypieaAccount, Equals, SkypieaAccount{Account: "0123456789", Alias: "awsalks", Label: "ALKS - Lab"})
}

func (s *S) Test_GetAccountsAdmin(c *C) {
	testServer.Response(202, nil, getAccounts)

	resp, err := s.client.GetAccounts()

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	var index = getIndexByAccount(resp.Accounts, "345678/ALKSAdmin - foobarbaz3")
	c.Assert(resp.Accounts[index].Account, Equals, "345678/ALKSAdmin - foobarbaz3") // make sure account name is transformed to key
	c.Assert(resp.Accounts[index].Role, Equals, "Admin")
	c.Assert(resp.Accounts[index].IamActive, Equals, true)
	c.Assert(resp.Accounts[index].SkypieaAccount, Equals, SkypieaAccount{Account: "0123456789", Alias: "awsalks", Label: "ALKS - Nonprod"})
}

var sessionCreate = `
{
    "accessKey": "foo",
    "secretKey": "bar",
    "sessionToken": "baz"
}
`

// this mapping is so dumb..
var getAccounts = `
{
	"StatusMessage": "Success",
	"accountListRole": {
		"123456/ALKSPowerUser - foobarbaz": [
		{
			"account": "123456/ALKSPowerUser",
			"role": "PowerUser",
			"iamKeyActive": false,
			"skypieaAccount": {
				"Account": "0123456789",
				"alias": "awsalks",
				"label": "ALKS - Prod"
			}
		}
		],
		"234567/ALKSIAMAdmin - foobarbaz2": [
		{
			"account": "234567/ALKSIAMAdmin",
			"role": "IAMAdmin",
			"iamKeyActive": true,
			"skypieaAccount": {
				"Account": "0123456789",
				"alias": "awsalks",
				"label": "ALKS - Lab"
			}
		}
		],
		"345678/ALKSAdmin - foobarbaz3": [
		{
			"account": "234567/ALKSAdmin",
			"role": "Admin",
			"iamKeyActive": true,
			"skypieaAccount": {
				"Account": "0123456789",
				"alias": "awsalks",
				"label": "ALKS - Nonprod"
			}
		}
		]
	}
}
`

var getNonIamLoginRoleResponse = `
{
		"requestId": "abcd1234",
		"statusMessage": "Success",
		"loginRole": {
				"account": "012345678910/ALKSAdmin",
				"role": "Admin",
				"iamKeyActive": true,
				"maxKeyDuration": 36
		}
}
`
