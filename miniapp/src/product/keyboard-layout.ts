import type { CSSProperties } from "react";

export function composerKeyboardStyle(keyboardHeight: number): CSSProperties | undefined {
  if (!Number.isFinite(keyboardHeight) || keyboardHeight <= 0) {
    return undefined;
  }
  return { paddingBottom: `${Math.ceil(keyboardHeight)}px` };
}
