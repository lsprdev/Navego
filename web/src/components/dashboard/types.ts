export type BrowserState = "running" | "starting" | "stopped" | "error";

export type BrowserInstance = {
  id: string;
  name: string;
  state: BrowserState;
  title: string;
  url: string;
  updatedAt: string;
};

export type ControlBrowser = {
  id: string;
  name: string;
  state: string;
  title: string;
  url: string;
  updated_at: string;
};

export type SessionUser = {
  id: string;
  name: string;
  email: string;
};

export type ActivityEvent = {
  id: string;
  event: string;
  result: "success" | "denied" | "error";
  browser_id?: string;
  browser_name?: string;
  created_at: string;
};

export type SavedCredential = {
  id: string;
  label: string;
  origin: string;
  username: string;
  updated_at: string;
};

export type SavedCredentialInput = {
  label: string;
  origin: string;
  username: string;
  password: string;
};
