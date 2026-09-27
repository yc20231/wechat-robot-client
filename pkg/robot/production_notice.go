package robot

import "errors"

// An empty list or a per-message rejection is not success, even if HTTP succeeded.
func ValidateProductionNoticeReceipt(response SendTextMessageResponse) error {
	if response.Ret != 0 || len(response.List) != 1 || response.List[0].Ret != 0 || response.List[0].NewMsgId <= 0 {
		return errors.New("production notice has no unambiguous successful protocol receipt")
	}
	return nil
}
