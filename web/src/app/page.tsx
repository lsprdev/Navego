import { LandingPage } from "@/components/landing/landing-page";
import { getSessionUser } from "@/lib/navego-server";

export default async function Home() {
  const user = await getSessionUser();
  return (
    <LandingPage
      authenticated={Boolean(user)}
      userName={user?.name}
    />
  );
}
