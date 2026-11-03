import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ConfigProvider } from "antd";
import { getAntThemeConfig } from "@/lib/app-theme";
import "@/styles/globals.css";
import "@/styles/hfp-design-system.css";
import AdminErrorsPage from "@/pages/admin/errors";

const dark = new URLSearchParams(location.search).has("dark");
document.documentElement.classList.toggle("dark", dark);
document.documentElement.dataset.theme = dark ? "dark" : "light";
const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
createRoot(document.getElementById("root")!).render(
    <ConfigProvider theme={getAntThemeConfig(dark)}><QueryClientProvider client={client}><AdminErrorsPage /></QueryClientProvider></ConfigProvider>,
);
