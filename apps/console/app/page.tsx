import { redirect } from "next/navigation";

import { auth } from "@/auth";
import { ManagerConsole } from "@/components/layout/manager-console";

export default async function Home() {
  const session = await auth();
  if (!session?.identity) redirect("/login");
  return <ManagerConsole />;
}
