export type InitialSecurityCopy = {
  accountTitleKey: "bootstrapTitle" | "passwordResetTitle" | "adminAccountTitle" | "userAccountTitle";
  readyDescriptionKey: "bootstrapReadyDescription" | "passwordResetReadyDescription" | "adminReadyDescription" | "userReadyDescription";
  usernamePlaceholderKey: "adminUsername" | "username";
  passwordPlaceholderKey: "adminPassword" | "password";
};

type InitialSecurityCopyInput = {
  role?: string | null;
  mustResetPassword: boolean;
};

export function isAdminRole(role?: string | null): boolean {
  return role === "admin" || role === "superadmin";
}

export function getInitialSecurityCopy({ role, mustResetPassword }: InitialSecurityCopyInput): InitialSecurityCopy {
  const isAdmin = isAdminRole(role);
  const isAdminPasswordReset = isAdmin && mustResetPassword;

  if (isAdminPasswordReset) {
    return {
      accountTitleKey: "bootstrapTitle",
      readyDescriptionKey: "bootstrapReadyDescription",
      usernamePlaceholderKey: "adminUsername",
      passwordPlaceholderKey: "adminPassword",
    };
  }

  if (mustResetPassword) {
    return {
      accountTitleKey: "passwordResetTitle",
      readyDescriptionKey: "passwordResetReadyDescription",
      usernamePlaceholderKey: "username",
      passwordPlaceholderKey: "password",
    };
  }

  return {
    accountTitleKey: isAdmin ? "adminAccountTitle" : "userAccountTitle",
    readyDescriptionKey: isAdmin ? "adminReadyDescription" : "userReadyDescription",
    usernamePlaceholderKey: isAdmin ? "adminUsername" : "username",
    passwordPlaceholderKey: isAdmin ? "adminPassword" : "password",
  };
}
