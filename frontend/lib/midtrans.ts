export type MidtransSnapConfiguration = {
  client_key: string;
  snap_url: string;
};

const allowedSnapURLs = new Set([
  "https://app.sandbox.midtrans.com/snap/snap.js",
  "https://app.midtrans.com/snap/snap.js",
]);

let activeSignature = "";
let loading: Promise<void> | null = null;

export async function loadMidtransSnap(configuration: MidtransSnapConfiguration): Promise<void> {
  const clientKey = configuration.client_key.trim();
  const snapURL = configuration.snap_url.trim();
  if (!clientKey || !allowedSnapURLs.has(snapURL)) {
    throw new Error("Konfigurasi Midtrans dari server tidak valid");
  }

  const signature = `${snapURL}\n${clientKey}`;
  if (activeSignature === signature && window.snap) return;
  if (activeSignature === signature && loading) return loading;

  document.querySelectorAll("script[data-bengkel-midtrans-snap]").forEach((node) => node.remove());
  window.snap = undefined;
  activeSignature = signature;
  loading = new Promise<void>((resolve, reject) => {
    const script = document.createElement("script");
    script.src = snapURL;
    script.async = true;
    script.dataset.bengkelMidtransSnap = "true";
    script.setAttribute("data-client-key", clientKey);
    script.onload = () => {
      loading = null;
      if (!window.snap) {
        activeSignature = "";
        reject(new Error("SDK Midtrans tidak berhasil dimuat"));
        return;
      }
      resolve();
    };
    script.onerror = () => {
      loading = null;
      activeSignature = "";
      script.remove();
      reject(new Error("SDK Midtrans tidak dapat dihubungi"));
    };
    document.head.appendChild(script);
  });
  return loading;
}
