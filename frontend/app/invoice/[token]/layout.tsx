import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Invoice Pembayaran",
  robots: { index: false, follow: false, nocache: true },
  referrer: "no-referrer",
};

export default function PublicInvoiceLayout({ children }: { children: React.ReactNode }) {
  return children;
}
