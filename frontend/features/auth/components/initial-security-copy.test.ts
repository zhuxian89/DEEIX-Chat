import { describe, expect, test } from "vitest";

import { getInitialSecurityCopy } from "./initial-security-copy";

describe("getInitialSecurityCopy", () => {
  test("普通用户被管理员重置密码时使用普通用户文案", () => {
    expect(getInitialSecurityCopy({ role: "user", mustResetPassword: true })).toEqual({
      accountTitleKey: "passwordResetTitle",
      readyDescriptionKey: "passwordResetReadyDescription",
      usernamePlaceholderKey: "username",
      passwordPlaceholderKey: "password",
    });
  });

  test("管理员初始化仍使用管理员文案", () => {
    expect(getInitialSecurityCopy({ role: "admin", mustResetPassword: true }).accountTitleKey).toBe("bootstrapTitle");
    expect(getInitialSecurityCopy({ role: "superadmin", mustResetPassword: true }).accountTitleKey).toBe("bootstrapTitle");
  });

  test("普通初始化和管理员普通设置不会被密码重置状态混淆", () => {
    expect(getInitialSecurityCopy({ role: "user", mustResetPassword: false }).accountTitleKey).toBe("userAccountTitle");
    expect(getInitialSecurityCopy({ role: "admin", mustResetPassword: false }).accountTitleKey).toBe("adminAccountTitle");
  });
});
