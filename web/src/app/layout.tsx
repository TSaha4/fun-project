import type { Metadata } from "next";
import type { ReactNode } from "react";
import "./globals.css";
import { ThemeProvider } from "@/lib/theme/ThemeProvider";
import { CompanionProvider } from "@/lib/companion/CompanionProvider";

export const metadata: Metadata = {
  title: "Local Capture Dashboard",
  description: "Local-first personal capture dashboard",
};

export default function RootLayout({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <html lang="en">
      <body><ThemeProvider><CompanionProvider>{children}</CompanionProvider></ThemeProvider></body>
    </html>
  );
}
