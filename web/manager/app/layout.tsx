import type { Metadata } from "next";

import { THEME_STORAGE_KEY } from "@/lib/theme";

import "./globals.css";

export const metadata: Metadata = {
  title: "SysArmor Manager",
  description: "Operator console for SysArmor manager workflows.",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const themeScript = `
    (() => {
      try {
        const stored = localStorage.getItem("${THEME_STORAGE_KEY}");
        const mode = stored === "light" || stored === "dark" || stored === "system" ? stored : "system";
        const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
        const resolved = mode === "system" ? (prefersDark ? "dark" : "light") : mode;
        document.documentElement.classList.toggle("dark", resolved === "dark");
        document.documentElement.dataset.theme = mode;
      } catch {
        document.documentElement.classList.add("dark");
        document.documentElement.dataset.theme = "system";
      }
    })();
  `;

  return (
    <html lang="zh-CN" suppressHydrationWarning>
      <body>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
        {children}
      </body>
    </html>
  );
}
