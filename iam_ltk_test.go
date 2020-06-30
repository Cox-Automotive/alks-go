package alks

import (
	. "github.com/motain/gocheck"
)

func (s *S) Test_GetLongTermKeys(c *C) {
	testServer.Response(202, nil, longTermKeys)

	resp, err := s.client.GetLongTermKeys()

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp.LongTermKeys, DeepEquals, []LongTermKey{{UserName: "bob", AccessKeyID: "verySecret", Status: "Active", CreateDate: "2020-04-20"}})
}

func (s *S) Test_GetLongTermKey(c *C) {
	testServer.Response(202, nil, longTermKey)

	resp, err := s.client.GetLongTermKey("bob")

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp.LongTermKey, DeepEquals, LongTermKey{})
}

func (s *S) Test_CreateLongTermKeys(c *C) {
	testServer.Response(202, nil, createLongTermKeys)

	resp, err := s.client.CreateLongTermKey("MY_USERNAME")

	c.Assert(err, IsNil)
	c.Assert(resp.CreateLongTermKey, DeepEquals, CreateLongTermKey{
		IAMUserName: "MY_USERNAME",
		IAMUserArn:  "arn:aws:iam::012345678910:user/acct-managed/MY_USERNAME",
		AccessKey:   "thisismykey",
		SecretKey:   "secret/thisismysecret",
	})
	c.Assert(resp.BaseLongTermKeyResponse, DeepEquals, BaseLongTermKeyResponse{
		Action:              "CREATE",
		AddedIAMUserToGroup: true,
		PartialError:        false,
	})
}

func (s *S) Test_DeleteLongTermKeys(c *C) {
	testServer.Response(202, nil, deleteLongTermKeys)

	resp, err := s.client.DeleteLongTermKey("MyUserName")

	c.Assert(err, IsNil)
	c.Assert(resp.BaseLongTermKeyResponse, DeepEquals, BaseLongTermKeyResponse{
		AddedIAMUserToGroup: false,
		PartialError:        false,
	})
}

var deleteLongTermKeys = `
{
  "addedIAMUserToGroup": false,
  "partialError": false
}
`

var createLongTermKeys = `
{
    "account": "012345678910/ALKSAdmin - AccountAlias",
    "action": "CREATE",
    "iamUserName": "MY_USERNAME",
    "iamUserArn": "arn:aws:iam::012345678910:user/acct-managed/MY_USERNAME",
    "addedIAMUserToGroup": true,
    "partialError": false,
	"accessKey": "thisismykey",
    "secretKey": "secret/thisismysecret"
}
`

var longTermKeys = `
{
	"longTermKeys": [
		{
			"userName": "bob",
			"accessKeyId": "verySecret",
			"status": "Active",
			"createDate": "2020-04-20"
		}
	]
}
`

var longTermKey = `
{
	"userName": "bob",
	"accessKeyId": "verySecret",
	"status": "Active",
	"createDate": "2020-04-20"
}
`
