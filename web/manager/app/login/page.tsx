import { LockKeyhole } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

import { loginAction } from "./actions";

export default async function LoginPage({
  searchParams,
}: {
  searchParams: Promise<{ error?: string }>;
}) {
  const failed = (await searchParams).error === "credentials";

  return (
    <main className="grid min-h-screen place-items-center bg-bg px-4">
      <section className="w-full max-w-sm rounded-lg border border-border bg-overlay p-6 shadow-sm">
        <div className="mb-6 flex items-center gap-3">
          <div className="grid size-9 place-items-center rounded-lg bg-primary text-primary-fg">
            <LockKeyhole className="size-5" />
          </div>
          <div>
            <h1 className="text-base font-semibold">SysArmor Manager</h1>
            <p className="text-sm text-muted-fg">管理员登录</p>
          </div>
        </div>
        <form action={loginAction} className="space-y-4">
          <label className="grid gap-1.5 text-sm font-medium">
            用户名
            <Input name="username" autoComplete="username" required />
          </label>
          <label className="grid gap-1.5 text-sm font-medium">
            密码
            <Input name="password" type="password" autoComplete="current-password" required />
          </label>
          {failed ? <p className="text-sm text-danger">用户名或密码错误</p> : null}
          <Button type="submit" className="w-full">登录</Button>
        </form>
      </section>
    </main>
  );
}
