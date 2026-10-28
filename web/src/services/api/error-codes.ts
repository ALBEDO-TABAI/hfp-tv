export const PLATFORM_ERROR_REASONS = {
    invalidCredentials: "invalid_credentials",
    rateLimited: "login_rate_limited",
    unauthenticated: "unauthenticated",
    adminRequired: "admin_required",
    invalidInput: "invalid_input",
    invalidPassword: "invalid_password",
    usernameTaken: "username_taken",
    invalidInvitation: "invalid_invitation",
    notFound: "not_found",
    csrfRejected: "csrf_rejected",
    internalError: "internal_error",
} as const;

export type PlatformErrorReason = typeof PLATFORM_ERROR_REASONS[keyof typeof PLATFORM_ERROR_REASONS];
