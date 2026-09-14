"use client";

import * as React from "react";
import { MapPinned, Monitor, Moon, Sun } from "lucide-react";
import { motion } from "motion/react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { LogoCarousel, type LogoCarouselLogo } from "@/components/ui/logo-carousel";
import { Onboarding } from "@/components/ui/onboarding";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { SpinnerLabel } from "@/components/ui/spinner";
import { dispatchUserProfileUpdated } from "@/features/settings/events/user-profile-events";
import {
  readLocalAppearancePreferences,
  serializeAppearancePreferences,
} from "@/features/settings/utils/appearance-preferences";
import {
  completeOnboarding,
  isPasswordReuseNotAllowedError,
  patchMe,
  patchUsername,
} from "@/shared/api/auth";
import type { UserDTO } from "@/shared/api/auth.types";
import {
  DISPLAY_NAME_MAX_LENGTH,
  PASSWORD_MIN_LENGTH,
  USERNAME_MAX_LENGTH,
  isDisplayNameLengthValid,
  isPasswordPolicyValid,
  isUsernamePolicyValid,
} from "@/shared/auth/account-policy";
import { useAuthSession } from "@/shared/auth/auth-session-context";
import { clearSessionAndRedirectToLogin } from "@/shared/auth/session";
import { useAppLocale } from "@/i18n/app-i18n-provider";
import { APP_LOCALE_LABELS, APP_LOCALES, type AppLocale } from "@/i18n/config";
import { useLocalizedErrorMessage } from "@/i18n/use-localized-error";
import { AppLogo } from "@/shared/components/app-logo";
import { TimeZoneSelect } from "@/shared/components/time-zone-select";
import { useTheme, type ThemePreset } from "@/shared/components/theme-provider";
import { detectCurrentTimeZone } from "@/shared/lib/time-zone";
import { cn } from "@/lib/utils";
import { getInitialSecurityCopy, isAdminRole } from "./initial-security-copy";

const ADMIN_ONBOARDING_TIPS = [
  "adminTips.upstreams",
  "adminTips.mcp",
  "adminTips.files",
  "adminTips.context",
  "adminTips.trace",
  "adminTips.billing",
  "adminTips.admin",
  "adminTips.ops",
];

const USER_ONBOARDING_TIPS = [
  "userTips.profile",
  "userTips.models",
  "userTips.files",
  "userTips.conversation",
];

function titleFromIconSlug(slug: string): string {
  return slug
    .replace(/-(brand|brand-color|color|text|text-cn)$/u, "")
    .split("-")
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

const ONBOARDING_LOGO_ITEMS: LogoCarouselLogo[] = [
  "openai",
  "codex",
  "anthropic",
  "claude",
  "google",
  "gemini",
  "gemma",
  "xai",
  "grok",
  "moonshot",
  "kimi",
  "alibaba",
  "alibabacloud",
  "qwen",
  "deepseek",
  "xiaomimimo",
  "zhipu",
  "chatglm",
  "minimax",
  "doubao",
  "mistral",
  "hunyuan",
  "longcat",
  "openrouter",
  "copilot",
  "replicate",
  "fal",
  "stability",
  "runway",
  "luma",
  "ideogram",
  "midjourney",
  "suno",
  "elevenlabs",
].map((slug, index) => ({
  id: `${slug}-${index}`,
  name: titleFromIconSlug(slug),
  src: `/vendor/lobehub-icons/${slug}.svg`,
}));

const ONBOARDING_THEME_PRESETS: ThemePreset[] = [
  "default",
  "azure",
  "cobalt",
  "graphite",
  "lagoon",
  "ink",
  "ochre",
  "sepia",
];

function OnboardingFeatureCarousel({
  activeIndex,
  logos,
  tips,
}: {
  activeIndex: number;
  logos: LogoCarouselLogo[];
  tips: string[];
}) {
  const t = useTranslations("guide");
  const activeTip = tips[activeIndex] ?? tips[0];

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="flex min-h-[238px] flex-1 items-center justify-center px-5">
        <LogoCarousel
          logos={logos}
          columnCount={3}
          className="w-full justify-center space-x-4"
          columnClassName="h-24 w-24 md:h-24 md:w-24"
          logoClassName="h-12 w-12 md:h-14 md:w-14"
        />
      </div>
      <div className="space-y-3 p-3">
        <div className="flex w-full gap-1.5 overflow-hidden">
          {tips.map((_, index) => (
            <div className="h-1 min-w-0 flex-1 overflow-hidden rounded-full bg-border/80" key={index}>
              {index === activeIndex ? (
                <motion.span
                  key={`onboarding-progress-${activeIndex}`}
                  className="block h-full origin-left rounded-full bg-foreground/75"
                  initial={{ scaleX: 0 }}
                  animate={{ scaleX: 1 }}
                  transition={{ duration: 4.2, ease: "linear" }}
                />
              ) : null}
            </div>
          ))}
        </div>
        <div className="flex h-[3.75rem] items-start overflow-hidden">
          <motion.p
            key={activeTip}
            className="text-xs font-medium leading-5 tracking-normal text-foreground"
            initial={{ opacity: 0, y: 4 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -4 }}
            transition={{ duration: 0.22, ease: "easeOut" }}
          >
            {t(activeTip)}
          </motion.p>
        </div>
      </div>
    </div>
  );
}

export function InitialSecurityGuard() {
  const t = useTranslations("guide");
  const tCommonErrors = useTranslations("common.errors");
  const resolveErrorMessage = useLocalizedErrorMessage();
  const { locale, setLocale } = useAppLocale();
  const { preset, setPreset, theme, setTheme } = useTheme();
  const { accessToken, user, refreshUser } = useAuthSession();
  const [viewer, setViewer] = React.useState<UserDTO | null>(null);
  const [step, setStep] = React.useState(1);
  const [activeTipIndex, setActiveTipIndex] = React.useState(0);
  const [guideActive, setGuideActive] = React.useState(false);
  const [username, setUsername] = React.useState("");
  const [displayName, setDisplayName] = React.useState("");
  const [timezone, setTimezone] = React.useState(detectCurrentTimeZone);
  const [password, setPassword] = React.useState("");
  const [savingAccount, setSavingAccount] = React.useState(false);
  const [savingLocale, setSavingLocale] = React.useState<AppLocale | null>(null);
  const [savingThemePreset, setSavingThemePreset] = React.useState(false);
  const [savingPersonalization, setSavingPersonalization] = React.useState(false);
  const [finishing, setFinishing] = React.useState(false);
  const initializedTimeZoneUserRef = React.useRef<string | null>(null);
  const isAdminGuide = isAdminRole(viewer?.role);
  const activeOnboardingTips = isAdminGuide ? ADMIN_ONBOARDING_TIPS : USER_ONBOARDING_TIPS;

  React.useEffect(() => {
    setViewer(user);
    setUsername(user?.username ?? "");
    setDisplayName(user?.displayName ?? "");
    if (!user) {
      setGuideActive(false);
      initializedTimeZoneUserRef.current = null;
      setStep(1);
      return;
    }
    if (user.initialSecurityRequired) {
      if (initializedTimeZoneUserRef.current !== user.publicID) {
        initializedTimeZoneUserRef.current = user.publicID;
        setTimezone(detectCurrentTimeZone());
      }
    } else {
      initializedTimeZoneUserRef.current = null;
      setTimezone(user.timezone.trim() || detectCurrentTimeZone());
    }
    setGuideActive(Boolean(user.initialSecurityRequired));
  }, [user]);

  React.useEffect(() => {
    if (!guideActive) {
      return;
    }

    const timer = window.setInterval(() => {
      setActiveTipIndex((current) => (current + 1) % activeOnboardingTips.length);
    }, 4200);

    return () => window.clearInterval(timer);
  }, [activeOnboardingTips.length, guideActive]);

  React.useEffect(() => {
    setActiveTipIndex(0);
  }, [isAdminGuide]);

  const mustResetPassword = Boolean(viewer?.mustResetPassword);
  const initialSecurityCopy = getInitialSecurityCopy({ role: viewer?.role, mustResetPassword });
  const currentTimeZone = React.useMemo(() => detectCurrentTimeZone(), []);
  const welcomeTitle = isAdminGuide ? t("adminWelcomeTitle") : t("userWelcomeTitle");
  const welcomeDescription = isAdminGuide
    ? t("adminWelcomeDescription")
    : t("userWelcomeDescription");
  const accountTitle = t(initialSecurityCopy.accountTitleKey);
  const readyDescription = t(initialSecurityCopy.readyDescriptionKey);

  const submitAccountStep = React.useCallback(async () => {
    if (!viewer?.initialSecurityRequired || savingAccount) return;
    const nextUsername = username.trim().toLowerCase();
    const nextDisplayName = displayName.trim();
    const nextPassword = password.trim();
    if (viewer.initialUsernameRequired && nextUsername === viewer.username.trim().toLowerCase()) {
      toast.error(t("toasts.changeInitialUsername"));
      return;
    }
    if (viewer.initialUsernameRequired && !isUsernamePolicyValid(nextUsername)) {
      toast.error(t("toasts.usernameTooShort"));
      return;
    }
    if (!viewer.mustResetPassword && !isDisplayNameLengthValid(nextDisplayName)) {
      toast.error(t("toasts.displayNameRequired"));
      return;
    }
    if (viewer.mustResetPassword && !isPasswordPolicyValid(nextPassword)) {
      toast.error(t("toasts.passwordTooShort"));
      return;
    }

    setSavingAccount(true);
    try {
      let nextViewer = viewer;
      if (viewer.initialUsernameRequired) {
        nextViewer = await patchUsername(accessToken, { username: nextUsername });
      }
      const profilePayload: Parameters<typeof patchMe>[1] = {};
      if (!viewer.mustResetPassword && nextDisplayName !== viewer.displayName.trim()) {
        profilePayload.displayName = nextDisplayName;
      }
      if (Object.keys(profilePayload).length > 0) {
        nextViewer = await patchMe(accessToken, profilePayload);
      }
      setViewer(nextViewer);
      dispatchUserProfileUpdated(nextViewer);
      setStep(3);
    } catch (error) {
      toast.error(t("toasts.saveAccountFailed"), {
        description: resolveErrorMessage(error, tCommonErrors("unknown")),
      });
    } finally {
      setSavingAccount(false);
    }
  }, [accessToken, displayName, password, resolveErrorMessage, savingAccount, t, tCommonErrors, username, viewer]);

  const handleLocaleChange = React.useCallback((nextLocale: AppLocale) => {
    if (nextLocale === locale) {
      return;
    }

    void setLocale(nextLocale);
  }, [locale, setLocale]);

  const saveWelcomeStep = React.useCallback(async () => {
    if (!viewer || savingLocale) {
      return;
    }
    const nextLocale = locale;

    if (nextLocale === viewer.locale.trim()) {
      setStep(2);
      return;
    }

    setSavingLocale(nextLocale);
    try {
      const nextViewer = await patchMe(accessToken, { locale: nextLocale });
      setViewer(nextViewer);
      dispatchUserProfileUpdated(nextViewer);
      setStep(2);
    } catch (error) {
      toast.error(t("toasts.saveLanguageFailed"), {
        description: resolveErrorMessage(error, tCommonErrors("unknown")),
      });
    } finally {
      setSavingLocale((current) => (current === nextLocale ? null : current));
    }
  }, [accessToken, locale, resolveErrorMessage, savingLocale, t, tCommonErrors, viewer]);

  const currentAppearancePreferences = React.useCallback(
    () => serializeAppearancePreferences({
      ...readLocalAppearancePreferences(),
      theme,
      preset,
    }),
    [preset, theme],
  );

  const saveThemePresetStep = React.useCallback(async () => {
    if (!viewer || savingThemePreset) return;
    const appearancePreferences = currentAppearancePreferences();

    if (appearancePreferences === (viewer.appearancePreferences?.trim() ?? "")) {
      setStep(4);
      return;
    }

    setSavingThemePreset(true);
    try {
      const nextViewer = await patchMe(accessToken, { appearancePreferences });
      setViewer(nextViewer);
      dispatchUserProfileUpdated(nextViewer);
      setStep(4);
    } catch (error) {
      toast.error(t("toasts.savePersonalizationFailed"), {
        description: resolveErrorMessage(error, tCommonErrors("unknown")),
      });
    } finally {
      setSavingThemePreset(false);
    }
  }, [accessToken, currentAppearancePreferences, resolveErrorMessage, savingThemePreset, t, tCommonErrors, viewer]);

  const savePersonalizationStep = React.useCallback(async () => {
    if (!viewer || savingPersonalization) return;
    const nextTimezone = timezone.trim() || currentTimeZone;
    const profilePayload: Parameters<typeof patchMe>[1] = {};
    const appearancePreferences = currentAppearancePreferences();

    if (nextTimezone !== (viewer.timezone.trim() || "Etc/UTC")) {
      profilePayload.timezone = nextTimezone;
    }
    if (appearancePreferences !== (viewer.appearancePreferences?.trim() ?? "")) {
      profilePayload.appearancePreferences = appearancePreferences;
    }

    if (Object.keys(profilePayload).length === 0) {
      setStep(5);
      return;
    }

    setSavingPersonalization(true);
    try {
      const nextViewer = await patchMe(accessToken, profilePayload);
      setViewer(nextViewer);
      dispatchUserProfileUpdated(nextViewer);
      setStep(5);
    } catch (error) {
      toast.error(t("toasts.savePersonalizationFailed"), {
        description: resolveErrorMessage(error, tCommonErrors("unknown")),
      });
    } finally {
      setSavingPersonalization(false);
    }
  }, [accessToken, currentAppearancePreferences, currentTimeZone, resolveErrorMessage, savingPersonalization, t, tCommonErrors, timezone, viewer]);

  const finishInitialSecurity = React.useCallback(async () => {
    if (!viewer || finishing) return;
    if (viewer.mustResetPassword && !isPasswordPolicyValid(password)) {
      toast.error(t("toasts.passwordTooShort"));
      setStep(2);
      return;
    }
    setFinishing(true);
    try {
      await refreshUser();
      const nextViewer = await completeOnboarding(
        accessToken,
        viewer.mustResetPassword ? { newPassword: password.trim() } : undefined,
      );
      if (nextViewer) {
        setViewer(nextViewer);
        dispatchUserProfileUpdated(nextViewer);
      }
      setGuideActive(false);
      if (viewer.mustResetPassword) {
        toast.success(t("toasts.initializedRelogin"));
        clearSessionAndRedirectToLogin();
        return;
      }
      toast.success(t("toasts.complete"));
    } catch (error) {
      if (isPasswordReuseNotAllowedError(error)) {
        setStep(2);
      }
      toast.error(t("toasts.completeFailed"), {
        description: resolveErrorMessage(error, tCommonErrors("unknown")),
      });
    } finally {
      setFinishing(false);
    }
  }, [accessToken, finishing, password, refreshUser, resolveErrorMessage, t, tCommonErrors, viewer]);

  if (!viewer || !guideActive) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-50 flex min-h-svh items-center justify-center overflow-y-auto bg-background/20 px-3 py-5 backdrop-blur-[2px]">
      <Onboarding
        value={step}
        onValueChange={setStep}
        totalSteps={5}
        role="dialog"
        aria-modal="true"
        aria-label={t("aria.onboarding")}
        className="grid w-full max-w-[820px] animate-in gap-0 overflow-hidden rounded-2xl border border-border/60 bg-background p-0 shadow-xl fade-in-0 zoom-in-95 duration-200 md:h-[430px] md:grid-cols-[0.95fr_1.05fr]"
      >
        <div className="hidden h-full flex-col bg-muted/15 p-4 md:flex">
          <OnboardingFeatureCarousel
            activeIndex={activeTipIndex}
            logos={ONBOARDING_LOGO_ITEMS}
            tips={activeOnboardingTips}
          />
        </div>

        <div className="flex h-full flex-col p-5">
          <div className="flex items-center justify-between gap-4">
            <AppLogo width={86} height={24} priority className="h-6 w-auto" />
            <Onboarding.StepIndicator variant="dots" dotClassName="bg-muted-foreground/25" />
          </div>

          <div className="flex flex-1">
          <Onboarding.Step step={1} className="flex flex-1 flex-col animate-in fade-in-0 slide-in-from-right-2 duration-200">
            <div className="flex flex-1 items-center">
              <div className="w-full space-y-6">
                <Onboarding.Header className="text-left">
                  <div className="space-y-3">
                    <h2 className="text-2xl font-semibold tracking-normal">{welcomeTitle}</h2>
                    <p className="text-sm leading-6 text-muted-foreground">
                      {welcomeDescription}
                    </p>
                  </div>
                </Onboarding.Header>

                <div className="w-full space-y-2">
                  <label className="text-xs font-medium text-muted-foreground" htmlFor="initial-locale-trigger">
                    {t("labels.language")}
                  </label>
                  <Select
                    value={locale}
                    disabled={Boolean(savingLocale)}
                    onValueChange={(value) => handleLocaleChange(value as AppLocale)}
                  >
                    <SelectTrigger id="initial-locale-trigger" aria-label={t("labels.language")} className="h-8 w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {APP_LOCALES.map((item) => (
                        <SelectItem key={item} value={item}>
                          {APP_LOCALE_LABELS[item]}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>
            </div>
            <Onboarding.Navigation aria-label={t("aria.welcomeNavigation")} className="mt-auto justify-end pt-6">
              <Button type="button" disabled={Boolean(savingLocale)} onClick={() => void saveWelcomeStep()}>
                {savingLocale ? <SpinnerLabel>{t("saving")}</SpinnerLabel> : t("start")}
              </Button>
            </Onboarding.Navigation>
          </Onboarding.Step>

          <Onboarding.Step step={2} className="flex flex-1 animate-in fade-in-0 slide-in-from-right-2 duration-200">
            <form
              className="flex flex-1 flex-col"
              autoComplete="on"
              onSubmit={(event) => {
                event.preventDefault();
                void submitAccountStep();
              }}
            >
              <div className="flex flex-1 items-center">
                <div className="w-full space-y-6">
                  <Onboarding.Header className="text-left">
                    <div className="space-y-2">
                      <h2 className="text-2xl font-semibold tracking-normal">
                        {accountTitle}
                      </h2>
                    </div>
                  </Onboarding.Header>

                  <div className="space-y-4">
                    <label className="block space-y-1.5" htmlFor="initial-username">
                      <span className="flex items-center text-xs font-medium">
                        {t("labels.username")}
                      </span>
                      <Input
                        id="initial-username"
                        name="username"
                        value={username}
                        onChange={(event) => setUsername(event.target.value.toLowerCase())}
                        disabled={savingAccount}
                        readOnly={!viewer.initialUsernameRequired}
                        maxLength={USERNAME_MAX_LENGTH}
                        autoComplete="username"
                        aria-disabled={!viewer.initialUsernameRequired}
                        placeholder={t(`placeholders.${initialSecurityCopy.usernamePlaceholderKey}`)}
                      />
                    </label>

                    {mustResetPassword ? (
                      <label className="block space-y-1.5" htmlFor="initial-password">
                        <span className="flex items-center text-xs font-medium">
                          {t("labels.password")}
                        </span>
                        <Input
                          id="initial-password"
                          name="password"
                          type="password"
                          value={password}
                          onChange={(event) => setPassword(event.target.value)}
                          disabled={savingAccount || !viewer.mustResetPassword}
                          autoComplete="new-password"
                          minLength={PASSWORD_MIN_LENGTH}
                          placeholder={t(`placeholders.${initialSecurityCopy.passwordPlaceholderKey}`)}
                        />
                      </label>
                    ) : (
                      <label className="block space-y-1.5" htmlFor="initial-display-name">
                        <span className="flex items-center text-xs font-medium">
                          {t("labels.displayName")}
                        </span>
                        <Input
                          id="initial-display-name"
                          name="name"
                          value={displayName}
                          onChange={(event) => setDisplayName(event.target.value)}
                          disabled={savingAccount}
                          maxLength={DISPLAY_NAME_MAX_LENGTH}
                          autoComplete="name"
                          placeholder={t("placeholders.displayName")}
                        />
                      </label>
                    )}

                  </div>
                </div>
              </div>

              <Onboarding.Navigation aria-label={t("aria.accountNavigation")} className="mt-auto justify-end pt-6">
                <Button type="button" variant="ghost" className="shadow-none" disabled={savingAccount} onClick={() => setStep(1)}>
                  {t("back")}
                </Button>
                <Button type="submit" disabled={savingAccount}>
                  {savingAccount ? <SpinnerLabel>{t("saving")}</SpinnerLabel> : t("continue")}
                </Button>
              </Onboarding.Navigation>
            </form>
          </Onboarding.Step>

          <Onboarding.Step step={3} className="flex flex-1 flex-col animate-in fade-in-0 slide-in-from-right-2 duration-200">
            <div className="flex flex-1 items-center">
              <div className="w-full space-y-5">
                <Onboarding.Header className="text-left">
                  <div className="space-y-2">
                    <h2 className="text-2xl font-semibold tracking-normal">{t("labels.themePreset")}</h2>
                    <p className="text-xs leading-5 text-muted-foreground">
                      {t("themePresetDescription")}
                    </p>
                  </div>
                </Onboarding.Header>

                <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                  {ONBOARDING_THEME_PRESETS.map((item) => (
                    <Button
                      key={item}
                      type="button"
                      variant="outline"
                      className={cn(
                        "h-8 justify-center px-2 text-xs shadow-none",
                        preset === item && "border-primary/45 bg-muted text-foreground hover:bg-muted hover:text-foreground",
                      )}
                      onClick={() => {
                        setPreset(item);
                      }}
                    >
                      {t(`themePreset.${item}`)}
                    </Button>
                  ))}
                </div>
              </div>
            </div>

            <Onboarding.Navigation aria-label={t("aria.themePresetNavigation")} className="mt-auto justify-end pt-6">
              <Button type="button" variant="ghost" className="shadow-none" disabled={savingThemePreset} onClick={() => setStep(2)}>
                {t("back")}
              </Button>
              <Button type="button" disabled={savingThemePreset} onClick={() => void saveThemePresetStep()}>
                {savingThemePreset ? <SpinnerLabel>{t("saving")}</SpinnerLabel> : t("continue")}
              </Button>
            </Onboarding.Navigation>
          </Onboarding.Step>

          <Onboarding.Step step={4} className="flex flex-1 flex-col animate-in fade-in-0 slide-in-from-right-2 duration-200">
            <div className="flex flex-1 items-center">
              <div className="w-full space-y-5">
                <Onboarding.Header className="text-left">
                  <div className="space-y-2">
                    <h2 className="text-2xl font-semibold tracking-normal">{t("personalizationTitle")}</h2>
                    <p className="text-xs leading-5 text-muted-foreground">
                      {t("personalizationDescription")}
                    </p>
                  </div>
                </Onboarding.Header>

                <div className="space-y-4">
                  <label className="block space-y-1.5" htmlFor="initial-timezone">
                    <span className="flex items-center text-xs font-medium">
                      {t("labels.region")}
                    </span>
                    <div className="flex gap-1.5">
                      <TimeZoneSelect
                        id="initial-timezone"
                        value={timezone || currentTimeZone}
                        disabled={savingPersonalization}
                        triggerClassName="h-8 min-w-0 flex-1 text-xs"
                        valueClassName="text-xs"
                        onChange={setTimezone}
                      />
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 shrink-0 shadow-none"
                        disabled={savingPersonalization || timezone === currentTimeZone}
                        onClick={() => setTimezone(currentTimeZone)}
                        aria-label={t("actions.syncTimezone")}
                        title={t("actions.syncTimezone")}
                      >
                        <MapPinned className="size-3.5 stroke-1" />
                      </Button>
                    </div>
                  </label>

                  <div className="space-y-1.5">
                    <span className="flex items-center text-xs font-medium">
                      {t("labels.theme")}
                    </span>
                    <div className="grid grid-cols-3 gap-2">
                      {([
                        ["light", Sun],
                        ["system", Monitor],
                        ["dark", Moon],
                      ] as const).map(([mode, Icon]) => (
                        <Button
                          key={mode}
                          type="button"
                          variant="outline"
                          className={cn(
                            "h-8 justify-center gap-1.5 px-2 text-xs shadow-none",
                            theme === mode && "border-foreground/80 bg-muted text-foreground hover:bg-muted hover:text-foreground",
                          )}
                          onClick={() => {
                            setTheme(mode);
                          }}
                        >
                          <Icon className="size-3.5 stroke-1" />
                          {t(`theme.${mode}`)}
                        </Button>
                      ))}
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <Onboarding.Navigation aria-label={t("aria.personalizationNavigation")} className="mt-auto justify-end pt-6">
              <Button type="button" variant="ghost" className="shadow-none" disabled={savingPersonalization} onClick={() => setStep(3)}>
                {t("back")}
              </Button>
              <Button type="button" disabled={savingPersonalization} onClick={() => void savePersonalizationStep()}>
                {savingPersonalization ? <SpinnerLabel>{t("saving")}</SpinnerLabel> : t("continue")}
              </Button>
            </Onboarding.Navigation>
          </Onboarding.Step>

          <Onboarding.Step step={5} className="flex flex-1 flex-col animate-in fade-in-0 slide-in-from-right-2 duration-200">
            <div className="flex flex-1 items-center">
              <div className="w-full space-y-5">
                <Onboarding.Header className="text-left">
                  <div className="space-y-2">
                    <h2 className="text-2xl font-semibold tracking-normal">{t("ready")}</h2>
                    <p className="text-xs text-muted-foreground">
                      {readyDescription}
                    </p>
                  </div>
                </Onboarding.Header>

              </div>
            </div>

            <Onboarding.Navigation aria-label={t("aria.finishNavigation")} className="mt-auto justify-end pt-6">
              <Button type="button" variant="ghost" className="shadow-none" disabled={finishing} onClick={() => setStep(4)}>
                {t("back")}
              </Button>
              <Button type="button" disabled={finishing} onClick={() => void finishInitialSecurity()}>
                {finishing ? <SpinnerLabel>{t("finishing")}</SpinnerLabel> : t("finish")}
              </Button>
            </Onboarding.Navigation>
          </Onboarding.Step>
          </div>
        </div>
      </Onboarding>
    </div>
  );
}
