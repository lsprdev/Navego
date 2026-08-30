import type { Metadata } from "next";
import { IBM_Plex_Mono, Instrument_Sans } from "next/font/google";

import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";

import "./globals.css";

const instrumentSans = Instrument_Sans({
  variable: "--font-instrument-sans",
  subsets: ["latin"],
});

const ibmPlexMono = IBM_Plex_Mono({
  variable: "--font-ibm-plex-mono",
  subsets: ["latin"],
  weight: ["400", "500", "600"],
});

export const metadata: Metadata = {
  title: {
    default: "Navego — A web trabalha. Você mantém o controle.",
    template: "%s · Navego",
  },
  description:
    "Dê ao ChatGPT um navegador real, persistente e visível, sem abrir mão do controle humano.",
  applicationName: "Navego",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html
      lang="pt-BR"
      className={`${instrumentSans.variable} ${ibmPlexMono.variable} dark h-full antialiased`}
    >
      <body className="flex min-h-full flex-col">
        <TooltipProvider>
          {children}
          <Toaster position="bottom-right" richColors />
        </TooltipProvider>
      </body>
    </html>
  );
}
