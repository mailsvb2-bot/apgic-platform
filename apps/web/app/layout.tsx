import "./globals.css";

export const metadata = {
  title: "APGIC",
  description: "APGIC Platform",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="ru">
      <body>{children}</body>
    </html>
  );
}
