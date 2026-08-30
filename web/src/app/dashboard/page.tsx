import { redirect } from "next/navigation";

import { Dashboard } from "@/components/dashboard/dashboard";
import { getBrowsers, getSessionUser } from "@/lib/navego-server";

export default async function DashboardPage({
  searchParams,
}: {
  searchParams: Promise<{ viewer?: string | string[] }>;
}) {
  const user = await getSessionUser();
  if (!user) redirect("/login");

  const browsers = await getBrowsers();
  const requestedViewer = (await searchParams).viewer;
  const initialViewerBrowserID =
    typeof requestedViewer === "string" &&
    browsers.some((browser) => browser.id === requestedViewer)
      ? requestedViewer
      : undefined;
  return (
    <Dashboard
      initialUser={user}
      initialBrowsers={browsers}
      initialViewerBrowserID={initialViewerBrowserID}
      mcpURL={
        process.env.NAVEGO_PUBLIC_MCP_URL?.trim() ||
        "https://mcp.browser.lspr.dev/mcp"
      }
    />
  );
}
