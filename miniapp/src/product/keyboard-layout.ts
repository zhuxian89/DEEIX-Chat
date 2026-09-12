import type { CSSProperties } from "react";

type KeyboardEvent = { detail: { height?: number } };

export function composerKeyboardHandlers(onHeightChange: (height: number) => void) {
  const updateHeight = (event: KeyboardEvent) => {
    const height = event.detail.height;
    // Android focus events can omit height; wait for keyboardheightchange then.
    if (typeof height === "number" && Number.isFinite(height)) {
      onHeightChange(Math.max(0, height));
    }
  };

  // Track the native textarea directly so layout does not depend on the
  // app-wide keyboard notification arriving when the input gains focus.
  return {
    onFocus: updateHeight,
    onKeyboardHeightChange: updateHeight,
    onBlur: () => onHeightChange(0),
  };
}

export function composerKeyboardStyle(keyboardHeight: number): CSSProperties | undefined {
  if (!Number.isFinite(keyboardHeight) || keyboardHeight <= 0) {
    return undefined;
  }
  return { paddingBottom: `${Math.ceil(keyboardHeight)}px` };
}
