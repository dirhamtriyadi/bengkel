import type { Metadata } from "next";
import "./globals.css";
import { Providers } from "@/components/providers";

export const metadata:Metadata={metadataBase:new URL(process.env.NEXT_PUBLIC_SITE_URL??"http://localhost:3000"),title:{default:"Bengkel Maju Motor",template:"%s | Bengkel Maju Motor"},description:"Operasional bengkel modern dan servis motor terpercaya.",robots:{index:true,follow:true}};
export default function RootLayout({children}:{children:React.ReactNode}){return <html lang="id"><body><Providers>{children}</Providers></body></html>}
