import type { ComponentProps, ReactNode } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, test, vi } from "vitest";

const { authApi, authSession, appLocale, themeState } = vi.hoisted(() => ({
  authApi: {
    completeOnboarding: vi.fn(),
    startCurrentTwoFactorSetup: vi.fn(),
  },
  authSession: {
    accessToken: "test-token",
    refreshUser: vi.fn(),
    user: null as Record<string, unknown> | null,
  },
  appLocale: {
    locale: "zh-CN",
    setLocale: vi.fn(),
  },
  themeState: {
    preset: "default",
    setPreset: vi.fn(),
    theme: "system",
    setTheme: vi.fn(),
  },
}));

vi.mock("next-intl", () => ({
  useTranslations: (namespace: string) => (key: string) => {
    if (namespace !== "guide") {
      return key;
    }

    const messages: Record<string, string> = {
      "adminWelcomeTitle": "欢迎进入 DEEIX Chat 管理端",
      "adminWelcomeDescription": "管理员设置",
      "adminAccountTitle": "完善管理账户",
      "adminTwoFactorTitle": "保护管理账户",
      "adminTips.upstreams": "管理功能",
      "aria.accountNavigation": "账户设置步骤导航",
      "aria.finishNavigation": "完成引导步骤导航",
      "aria.onboarding": "引导",
      "aria.personalizationNavigation": "个性化步骤导航",
      "aria.themePresetNavigation": "主题选择步骤导航",
      "aria.twoFactorNavigation": "两步验证步骤导航",
      "aria.twoFactorQRCode": "两步验证二维码",
      "aria.welcomeNavigation": "欢迎步骤导航",
      "back": "返回",
      "bootstrapTitle": "初始化管理员账户",
      "continue": "继续",
      "finish": "完成",
      "finishing": "完成中",
      "generating": "生成中",
      "labels.language": "语言",
      "labels.password": "密码",
      "passwordResetTitle": "设置新密码",
      "placeholders.adminPassword": "设置管理员密码",
      "placeholders.adminUsername": "设置管理员用户名",
      "placeholders.password": "设置新密码",
      "placeholders.username": "设置用户名",
      "processing": "处理中",
      "ready": "已就绪",
      "saving": "保存中",
      "start": "开始",
      "userTips.profile": "个人设置",
      "userWelcomeDescription": "用户设置",
      "userWelcomeTitle": "欢迎使用 DEEIX Chat",
    };

    return messages[key] ?? key;
  },
}));

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("motion/react", () => ({ motion: { p: "p", span: "span" } }));
vi.mock("@/shared/auth/auth-session-context", () => ({ useAuthSession: () => authSession }));
vi.mock("@/i18n/app-i18n-provider", () => ({ useAppLocale: () => appLocale }));
vi.mock("@/shared/components/theme-provider", () => ({ useTheme: () => themeState }));
vi.mock("@/i18n/use-localized-error", () => ({ useLocalizedErrorMessage: () => () => "unknown" }));
vi.mock("@/shared/auth/session", () => ({ clearSessionAndRedirectToLogin: vi.fn() }));
vi.mock("@/features/settings/events/user-profile-events", () => ({ dispatchUserProfileUpdated: vi.fn() }));
vi.mock("@/features/settings/utils/appearance-preferences", () => ({
  readLocalAppearancePreferences: () => ({}),
  serializeAppearancePreferences: () => "",
}));
vi.mock("@/shared/api/auth", () => ({
  completeOnboarding: authApi.completeOnboarding,
  isPasswordReuseNotAllowedError: () => false,
  patchMe: vi.fn(),
  patchUsername: vi.fn(),
  startCurrentTwoFactorSetup: authApi.startCurrentTwoFactorSetup,
}));
vi.mock("@/shared/auth/account-policy", () => ({
  DISPLAY_NAME_MAX_LENGTH: 64,
  PASSWORD_MIN_LENGTH: 8,
  USERNAME_MAX_LENGTH: 32,
  isDisplayNameLengthValid: () => true,
  isPasswordPolicyValid: () => true,
  isUsernamePolicyValid: () => true,
}));
vi.mock("@/shared/lib/time-zone", () => ({ detectCurrentTimeZone: () => "Etc/UTC" }));
vi.mock("@/shared/components/app-logo", () => ({ AppLogo: () => <div /> }));
vi.mock("@/shared/components/time-zone-select", () => ({ TimeZoneSelect: () => <select /> }));
vi.mock("@/components/ui/logo-carousel", () => ({ LogoCarousel: () => <div /> }));
vi.mock("@/components/ui/spinner", () => ({ SpinnerLabel: ({ children }: { children?: ReactNode }) => <span>{children}</span> }));
vi.mock("@/components/ui/button", () => ({
  Button: ({ children, ...props }: ComponentProps<"button">) => <button {...props}>{children}</button>,
}));
vi.mock("@/components/ui/input", () => ({
  Input: (props: ComponentProps<"input">) => <input {...props} />,
}));
vi.mock("@/components/ui/select", () => ({
  Select: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  SelectContent: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  SelectItem: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  SelectTrigger: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  SelectValue: () => null,
}));

import { InitialSecurityGuard } from "./initial-security-guard";

afterEach(() => {
  cleanup();
  authSession.user = null;
  vi.clearAllMocks();
});

function makeUser(role: string, mustResetPassword: boolean) {
  return {
    appearancePreferences: "",
    avatarURL: "",
    createdAt: "",
    displayName: "",
    email: "user@example.com",
    emailBootstrapUsedAt: null,
    emailSource: "",
    emailVerifiedAt: null,
    id: 1,
    identityProviders: [],
    initialSecurityRequired: true,
    initialUsernameRequired: false,
    lastActiveAt: null,
    lastLoginAt: null,
    locale: "zh-CN",
    mustResetPassword,
    onboardingCompletedAt: null,
    passwordEnabled: true,
    passwordOrigin: "password",
    passwordSetAt: null,
    phone: "",
    phoneVerifiedAt: null,
    profilePreferences: "",
    publicID: "user_public_id",
    role,
    status: "active",
    subscriptionExpiresAt: null,
    subscriptionPlanID: null,
    subscriptionPlanName: "",
    subscriptionStatus: "active",
    subscriptionTier: "free",
    timezone: "Etc/UTC",
    twoFactorAvailable: false,
    twoFactorEnabled: false,
    twoFactorRecoveryCount: 0,
    twoFactorRequired: false,
    updatedAt: "",
    username: "user",
    usernameChangedAt: null,
  };
}

async function renderAccountStep(role: string, mustResetPassword: boolean) {
  authSession.user = makeUser(role, mustResetPassword);
  render(<InitialSecurityGuard />);
  await waitFor(() => expect(screen.getByRole("button", { name: "开始" })).toBeTruthy());
  fireEvent.click(screen.getByRole("button", { name: "开始" }));
  await waitFor(() => expect(screen.getByRole("heading")).toBeTruthy());
}

describe("InitialSecurityGuard account copy", () => {
  test("普通用户密码被重置后渲染设置新密码文案", async () => {
    await renderAccountStep("user", true);

    expect(screen.getByRole("heading", { name: "设置新密码" })).toBeTruthy();
    expect(screen.getByPlaceholderText("设置新密码")).toBeTruthy();
    expect(screen.queryByText("初始化管理员账户")).toBeNull();
  });

  test("管理员初始化仍渲染管理员文案", async () => {
    await renderAccountStep("admin", true);

    expect(screen.getByRole("heading", { name: "初始化管理员账户" })).toBeTruthy();
    expect(screen.getByPlaceholderText("设置管理员密码")).toBeTruthy();
  });
});

describe("InitialSecurityGuard steps", () => {
  test("账户步骤后直接进入主题设置且不启动两步验证", async () => {
    authSession.user = {
      ...makeUser("user", false),
      twoFactorAvailable: true,
    };
    authApi.startCurrentTwoFactorSetup.mockResolvedValue({
      otpauthURL: "otpauth://totp/example",
      secret: "secret",
    });

    render(<InitialSecurityGuard />);
    await waitFor(() => expect(screen.getByRole("button", { name: "开始" })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "开始" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "继续" })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "继续" }));

    await waitFor(() => expect(screen.getByRole("heading", { name: "labels.themePreset" })).toBeTruthy());
    expect(screen.getByRole("progressbar").getAttribute("aria-valuemax")).toBe("5");
    expect(authApi.startCurrentTwoFactorSetup).not.toHaveBeenCalled();
  });

  test("完成引导前仍刷新当前用户", async () => {
    authSession.user = makeUser("user", false);
    authSession.refreshUser.mockResolvedValue(authSession.user);
    authApi.completeOnboarding.mockResolvedValue({
      ...authSession.user,
      initialSecurityRequired: false,
    });

    render(<InitialSecurityGuard />);
    await waitFor(() => expect(screen.getByRole("button", { name: "开始" })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "开始" }));

    for (const heading of ["labels.themePreset", "personalizationTitle", "已就绪"]) {
      await waitFor(() => expect(screen.getByRole("button", { name: "继续" })).toBeTruthy());
      fireEvent.click(screen.getByRole("button", { name: "继续" }));
      await waitFor(() => expect(screen.getByRole("heading", { name: heading })).toBeTruthy());
    }

    fireEvent.click(screen.getByRole("button", { name: "完成" }));
    await waitFor(() => expect(authApi.completeOnboarding).toHaveBeenCalledTimes(1));

    expect(authSession.refreshUser).toHaveBeenCalledTimes(1);
    expect(authSession.refreshUser.mock.invocationCallOrder[0]).toBeLessThan(
      authApi.completeOnboarding.mock.invocationCallOrder[0],
    );
  });
});
