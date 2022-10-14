package alks

import (
	. "github.com/motain/gocheck"
)

func (s *S) Test_GetIamUsers(c *C) {
	testServer.Response(202, nil, iamUsers)

	resp, err := s.client.GetIamUsers()

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp.IamUsers, DeepEquals, []AllIamUsersResponseType{{UserName: "bob", AccessKeyID: "verySecret", Status: "Active", CreateDate: "2020-04-20"}})
}

func (s *S) Test_GetIamUserNoTags(c *C) {
	testServer.Response(202, nil, iamUser)

	resp, err := s.client.GetIamUser("bob")

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp.User, DeepEquals, IamUser{ARN: "arn.this.is.an.arn", AccountId: "123456789123", UserName: "bob", AccessKey: "thisIsAKey"})

}


func (s *S) Test_GetLongTermKeyWithTags(c *C) {
	testServer.Response(202, nil, iamUserWithTags)

	resp, err := s.client.GetIamUser("bob")

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp.User.Tags[0].Key, Equals, "foo")
	c.Assert(resp.User.Tags[0].Value, Equals, "bar")
	c.Assert(resp.User.Tags[1].Key, Equals, "cloud")
	c.Assert(resp.User.Tags[1].Value, Equals, "railway")
}

func (s *S) Test_CreateIamUserNoIamUserName(c *C) {
	tags := []Tag{
		{
			Key:   "foo",
			Value: "bar",
		},
		{
			Key:   "cloud",
			Value: "railway",
		},
	}

	options := IamUserOptions{
		Tags: &tags,
	}

	resp, err := s.client.CreateIamUser(&options)

	c.Assert(err, NotNil)
	c.Assert(resp, IsNil)
}

func (s *S) Test_CreateLongTermKeysNoTags(c *C) {
	testServer.Response(202, nil, createIamUser)

	username := "MY_USERNAME"

	options := IamUserOptions{
		IamUserName: &username,
	}

	resp, err := s.client.CreateIamUser(&options)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp.CreateIamUserApiResponse, DeepEquals, CreateIamUserApiResponse{
		IAMUserName: "MY_USERNAME",
		IAMUserArn:  "arn:aws:iam::012345678910:user/acct-managed/MY_USERNAME",
		AccessKey:   "thisismykey",
		SecretKey:   "secret/thisismysecret",
	})
	c.Assert(resp.BaseIamUserResponse, DeepEquals, BaseIamUserResponse{
		Action:              "CREATE",
		AddedIAMUserToGroup: true,
		PartialError:        false,
	})
}


func (s *S) Test_CreateIamUserWithTags(c *C) {
	testServer.Response(202, nil, createIamUserWithTags)

	username := "MY_USERNAME"

	tags := []Tag{
		{
			Key:   "foo",
			Value: "bar",
		},
		{
			Key:   "cloud",
			Value: "railway",
		},
	}

	options := IamUserOptions{
		IamUserName: &username,
		Tags:        &tags,
	}

	resp, err := s.client.CreateIamUser(&options)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp.CreateIamUserApiResponse, DeepEquals, CreateIamUserApiResponse{
		IAMUserName: "MY_USERNAME",
		IAMUserArn:  "arn:aws:iam::012345678910:user/acct-managed/MY_USERNAME",
		AccessKey:   "thisismykey",
		SecretKey:   "secret/thisismysecret",
	})
	c.Assert(resp.BaseIamUserResponse, DeepEquals, BaseIamUserResponse{
		Action:              "CREATE",
		AddedIAMUserToGroup: true,
		PartialError:        false,
	})
}

func (s *S) Test_UpdateIamUserWithTags(c *C) {
	testServer.Response(202, nil, iamUserWithTags)

	username := "MY_USERNAME"

	tags := []Tag{
		{
			Key:   "foo",
			Value: "bar",
		},
		{
			Key:   "cloud",
			Value: "railway",
		},
	}

	options := IamUserOptions{
		IamUserName: &username,
		Tags:        &tags,
	}

	resp, err := s.client.UpdateIamUser(&options)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp.User.Tags[0].Key, Equals, "foo")
	c.Assert(resp.User.Tags[0].Value, Equals, "bar")
	c.Assert(resp.User.Tags[1].Key, Equals, "cloud")
	c.Assert(resp.User.Tags[1].Value, Equals, "railway")
}

func (s *S) Test_UpdateIamUserWithTagsEmptyList(c *C) {
	testServer.Response(202, nil, updateIamUserNoTags)

	username := "MY_USERNAME"

	tags := []Tag{}

	options := IamUserOptions{
		IamUserName: &username,
		Tags:        &tags,
	}

	resp, err := s.client.UpdateIamUser(&options)

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
}

func (s *S) Test_UpdateIamUserNoTags(c *C) {
	username := "MY_USERNAME"

	options := IamUserOptions{
		IamUserName: &username,
	}

	resp, err := s.client.UpdateIamUser(&options)

	c.Assert(err, NotNil)
	c.Assert(resp, IsNil)
}

func (s *S) Test_DeleteLongTermKeys(c *C) {
	testServer.Response(202, nil, deleteIamUser)

	resp, err := s.client.DeleteIamUser("MyUserName")

	_ = testServer.WaitRequest()

	c.Assert(err, IsNil)
	c.Assert(resp.BaseIamUserResponse, DeepEquals, BaseIamUserResponse{
		AddedIAMUserToGroup: false,
		PartialError:        false,
	})
}

var deleteIamUser = `
{
  "addedIAMUserToGroup": false,
  "partialError": false
}
`


var iamUsers = `
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

var updateIamUserNoTags = `
{
	"item": {
		"arn": "arn.this.is.an.arn",
		"accountId": "123456789123",
		"userName": "bob",
		"accessKey": "thisIsAKey"
	}
}
`


var iamUser = `
{
	"item": {
		"arn": "arn.this.is.an.arn",
		"accountId": "123456789123",
		"userName": "bob",
		"accessKey": "thisIsAKey"
	}
}
`

var iamUserWithTags = `
{
	"item": {
		"arn": "arn.this.is.an.arn",
		"accountId": "123456789123",
		"userName": "bob",
		"accessKey": "thisIsAKey",
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
}
`

var longTermKeyWithTags = `
{
	"userName": "bob",
	"accessKeyId": "verySecret",
	"status": "Active",
	"createDate": "2020-04-20",
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

var createIamUser = `
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

var createIamUserWithTags = `
{
    "account": "012345678910/ALKSAdmin - AccountAlias",
    "action": "CREATE",
    "iamUserName": "MY_USERNAME",
    "iamUserArn": "arn:aws:iam::012345678910:user/acct-managed/MY_USERNAME",
    "addedIAMUserToGroup": true,
    "partialError": false,
	"accessKey": "thisismykey",
    "secretKey": "secret/thisismysecret",
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
