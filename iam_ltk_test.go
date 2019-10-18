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
