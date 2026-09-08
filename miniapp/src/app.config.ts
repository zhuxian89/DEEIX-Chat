export default defineAppConfig({
  pages: ["pages/index/index"],
  permission: { "scope.record": { desc: "用于将你的语音转换成输入框文字" } },
  window: {
    backgroundTextStyle: "light",
    backgroundColor: "#f4f7f6",
    navigationBarBackgroundColor: "#ffffff",
    navigationBarTitleText: "AI省着用",
    navigationBarTextStyle: "black",
  },
});
