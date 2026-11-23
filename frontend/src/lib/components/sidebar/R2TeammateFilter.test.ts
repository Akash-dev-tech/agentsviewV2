// @vitest-environment jsdom
import { fireEvent, render, screen } from "@testing-library/svelte";
import { expect, it, vi } from "vite-plus/test";
import R2TeammateFilter from "./R2TeammateFilter.svelte";

const { selectUser, getTeammates } = vi.hoisted(() => ({
  selectUser: vi.fn(),
  getTeammates: vi.fn(),
}));

vi.mock("../../stores/sessions.svelte.js", () => ({
  sessions: { filters: {}, sessions: [], setR2UserFilter: selectUser },
}));

vi.mock("../../api/generated/index.js", () => ({
  SessionsService: { getApiV1R2Teammates: getTeammates },
}));

it("finds teammates by name or email and selects their user ID", async () => {
  getTeammates.mockResolvedValue({
    configured: true,
    users: [
      { user_id: "usr_sam", name: "Sam Lee", email: "sam@example.net" },
      { user_id: "usr_jo", name: "Jo Patel", email: "analysis@example.org" },
    ],
  });
  render(R2TeammateFilter);
  const trigger = await screen.findByRole("button", { name: /all users/i });
  await fireEvent.click(trigger);
  const search = screen.getByRole("combobox");
  await fireEvent.input(search, { target: { value: "patel" } });
  expect(screen.getByRole("option", { name: "Jo Patel (analysis@example.org)" })).toBeTruthy();
  expect(screen.queryByRole("option", { name: "Sam Lee (sam@example.net)" })).toBeNull();
  await fireEvent.input(search, { target: { value: "sam@example.net" } });
  await fireEvent.mouseDown(screen.getByRole("option", { name: "Sam Lee (sam@example.net)" }));
  expect(selectUser).toHaveBeenLastCalledWith("usr_sam");

  await fireEvent.click(trigger);
  await fireEvent.mouseDown(screen.getByRole("option", { name: "All users" }));
  expect(selectUser).toHaveBeenLastCalledWith("");
});
