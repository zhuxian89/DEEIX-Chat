package telegram

// NotifyMiniAppRegistration reports an account created by the first WeChat login.
func (n *Notifier) NotifyMiniAppRegistration(userID uint, username string) {
	n.notifyRegistration("小程序", userID, username, "")
}
