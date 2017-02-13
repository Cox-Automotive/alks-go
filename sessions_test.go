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

func (s *S) Test_GetAccountsPowerUser(c *C) {
	testServer.Response(202, nil, getAccounts)

	resp, err := s.client.GetAccounts()

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.Accounts[0].Account, Equals, "123456/ALKSPowerUser - foobarbaz") // make sure account name is transformed to key
	c.Assert(resp.Accounts[0].Role, Equals, "PowerUser")
	c.Assert(resp.Accounts[0].IamActive, Equals, false)
}

func (s *S) Test_GetAccountsIAMAdmin(c *C) {
	testServer.Response(202, nil, getAccounts)

	resp, err := s.client.GetAccounts()

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.Accounts[1].Account, Equals, "234567/ALKSIAMAdmin - foobarbaz2") // make sure account name is transformed to key
	c.Assert(resp.Accounts[1].Role, Equals, "IAMAdmin")
	c.Assert(resp.Accounts[1].IamActive, Equals, true)
}

func (s *S) Test_GetAccountsAdmin(c *C) {
	testServer.Response(202, nil, getAccounts)

	resp, err := s.client.GetAccounts()

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.Accounts[2].Account, Equals, "345678/ALKSAdmin - foobarbaz3") // make sure account name is transformed to key
	c.Assert(resp.Accounts[2].Role, Equals, "Admin")
	c.Assert(resp.Accounts[2].IamActive, Equals, true)
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
	"accountListRole": {
		"123456/ALKSPowerUser - foobarbaz": [
		{
			"account": "123456/ALKSPowerUser",
			"role": "PowerUser",
			"iamKeyActive": false
		}
		],
		"234567/ALKSIAMAdmin - foobarbaz2": [
		{
			"account": "234567/ALKSIAMAdmin",
			"role": "IAMAdmin",
			"iamKeyActive": true
		}
		],
		"345678/ALKSAdmin - foobarbaz3": [
		{
			"account": "234567/ALKSAdmin",
			"role": "Admin",
			"iamKeyActive": true
		}
		]
	}
}
`