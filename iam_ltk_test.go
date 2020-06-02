package alks

import (
	. "github.com/motain/gocheck"
)

func (s *S) Test_GetLongTermKeys(c *C) {
	testServer.Response(202, nil, longTermKeys)

	resp, err := s.client.GetLongTermKeys("012345678910", "myRole")

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp.LongTermKeys, DeepEquals, []LongTermKey{LongTermKey{UserName: "bob", AccessKeyID: "verySecret", Status: "Active", CreateDate: "2020-04-20"}})
}

func (s *S) Test_CreateLongTermKeys(c *C) {
	testServer.Response(202, nil, createLongTermKeys)

	resp, err := s.client.CreateLongTermKey("012345678910", "Admin", "AccountAlias", "MY_USERNAME")

	c.Assert(err, IsNil)
	c.Assert(resp.CreateLongTermKey, DeepEquals, CreateLongTermKey{
		Account:             "012345678910/ALKSAdmin - AccountAlias",
		Action:              "CREATE",
		IAMUserName:         "MY_USERNAME",
		IAMUserArn:          "arn:aws:iam::012345678910:user/acct-managed/MY_USERNAME",
		AddedIAMUserToGroup: true,
		PartialError:        false,
		AccessKey:           "thisismykey",
		SecretKey:           "secret/thisismysecret",
	})
}

func (s *S) Test_DeleteLongTermKeys(c *C) {
	testServer.Response(202, nil, deleteLongTermKeys)

	resp, err := s.client.DeleteLongTermKey("012345678910", "myRole", "accountAlias", "MyUserName")

	c.Assert(err, IsNil)
	c.Assert(resp.DeleteLongTermKey, DeepEquals, DeleteLongTermKey{
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
