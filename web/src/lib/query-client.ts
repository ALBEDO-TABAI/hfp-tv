import { QueryClient } from "@tanstack/react-query";
import { subscribeUserScope } from "@/lib/user-scope";

export const appQueryClient = new QueryClient({
    defaultOptions: {
        queries: {
            staleTime: 30_000,
            retry: false,
            refetchOnWindowFocus: false,
        },
    },
});

subscribeUserScope(() => appQueryClient.clear());
