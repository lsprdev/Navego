import { redirect } from "next/navigation";

import { Dashboard } from "@/components/dashboard/dashboard";
import { getBrowsers, getSessionUser } from "@/lib/navego-server";

export default async function DashboardPage() {
  const user = await getSessionUser();
  if (!user) redirect("/login");

  const browsers = await getBrowsers();
  return (
    <Dashboard
      initialUser={user}
      initialBrowsers={browsers}
      mcpURL={
        process.env.NAVEGO_PUBLIC_MCP_URL?.trim() ||
        "https://mcp.browser.lspr.dev/mcp"
      }
    />
  );
}
