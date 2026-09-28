import "./globals.css";

export const metadata = {
  title: "APGIC — подобрать специалиста под ваш запрос",
  description: "Опишите ситуацию своими словами, подтвердите запрос и выберите подходящего специалиста и время.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="ru">
      <body>{children}</body>
    </html>
  );
}
