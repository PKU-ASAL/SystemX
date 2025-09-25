import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import Script from "next/script";
import { ThemeProvider } from "next-themes";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "SysArmor 安全监控中心",
  description: "SysArmor EDR/HIDS 系统 - 实时监控系统安全状态和威胁情报",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <meta name="google" content="notranslate" />
        <meta name="translate" content="no" />
      </head>
      <body
        className={`${geistSans.variable} ${geistMono.variable} antialiased`}
      >
        <Script
          id="monkeyPatchDOMOperations"
          strategy="beforeInteractive"
          dangerouslySetInnerHTML={{
            __html: `
              (function() {
                // Patch removeChild
                const originalRemoveChild = Node.prototype.removeChild;
                Node.prototype.removeChild = function(child) {
                  try {
                    return originalRemoveChild.call(this, child);
                  } catch (err) {
                    if (
                      err instanceof Error &&
                      /not a child of this node/.test(err.message)
                    ) {
                      console.warn('⚠️ [DOM-PATCH] Ignored removeChild error:', child);
                      return child;
                    }
                    throw err;
                  }
                };
                
                // Patch insertBefore
                const originalInsertBefore = Node.prototype.insertBefore;
                Node.prototype.insertBefore = function(newNode, referenceNode) {
                  try {
                    return originalInsertBefore.call(this, newNode, referenceNode);
                  } catch (err) {
                    if (
                      err instanceof Error &&
                      (/not a child of this node/.test(err.message) || 
                       /The node before which the new node is to be inserted is not a child of this node/.test(err.message))
                    ) {
                      console.warn('⚠️ [DOM-PATCH] Ignored insertBefore error:', { newNode, referenceNode });
                      // 尝试直接appendChild作为回退
                      try {
                        return this.appendChild(newNode);
                      } catch (appendErr) {
                        console.warn('⚠️ [DOM-PATCH] appendChild fallback also failed:', appendErr);
                        return newNode;
                      }
                    }
                    throw err;
                  }
                };
                
                // Patch appendChild (for extra safety)
                const originalAppendChild = Node.prototype.appendChild;
                Node.prototype.appendChild = function(child) {
                  try {
                    return originalAppendChild.call(this, child);
                  } catch (err) {
                    if (
                      err instanceof Error &&
                      /Failed to execute 'appendChild'/.test(err.message)
                    ) {
                      console.warn('⚠️ [DOM-PATCH] Ignored appendChild error:', child);
                      return child;
                    }
                    throw err;
                  }
                };
                
                console.log('✅ [DOM-PATCH] Extended DOM operations monkey patch applied');
              })();
            `,
          }}
        />
        <ThemeProvider
          attribute="class"
          defaultTheme="system"
          enableSystem
          disableTransitionOnChange
        >
          {children}
        </ThemeProvider>
      </body>
    </html>
  );
}
