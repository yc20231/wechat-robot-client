package robot

import "testing"

func TestValidateProductionNoticeReceipt(t *testing.T) {
	good := SendTextMessageResponse{List: []TextMessageResponse{{Ret: 0, NewMsgId: 123}}}
	if err := ValidateProductionNoticeReceipt(good); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []SendTextMessageResponse{{}, {List: []TextMessageResponse{{Ret: 1, NewMsgId: 123}}}, {List: []TextMessageResponse{{Ret: 0}}}, {List: []TextMessageResponse{{NewMsgId: 1}, {NewMsgId: 2}}}} {
		if ValidateProductionNoticeReceipt(bad) == nil {
			t.Fatal("false successful receipt")
		}
	}
}
