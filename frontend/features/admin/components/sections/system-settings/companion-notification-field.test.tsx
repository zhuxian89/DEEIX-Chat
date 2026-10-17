import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";

import { CompanionNotificationField } from "./companion-notification-field";

afterEach(cleanup);

test("edits notification fields without losing the rest of the saved template", () => {
  const config = { enabled: true, templateId: "saved-template", titleField: "thing5", timeField: "time30", state: "formal" };
  const onChange = vi.fn();
  render(<CompanionNotificationField value={JSON.stringify(config)} onChange={onChange} />);
  fireEvent.change(screen.getByLabelText("日程标题字段"), { target: { value: "thing1" } });
  expect(JSON.parse(onChange.mock.calls[0][0])).toEqual({ ...config, titleField: "thing1" });
  fireEvent.click(screen.getByRole("switch"));
  expect(JSON.parse(onChange.mock.calls[1][0])).toEqual({ ...config, enabled: false });
  expect(screen.getByText("正式版")).toBeTruthy();
});

test("allows repairing malformed stored configuration without silently replacing it", () => {
  const onChange = vi.fn();
  render(<CompanionNotificationField value="broken" onChange={onChange} />);
  const field = screen.getByRole("textbox");
  expect((field as HTMLTextAreaElement).value).toBe("broken");
  fireEvent.change(field, { target: { value: "{}" } });
  expect(onChange).toHaveBeenCalledWith("{}");
});
