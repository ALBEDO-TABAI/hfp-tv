import "@fontsource-variable/inter";
import "@fontsource-variable/jetbrains-mono";
import "@/styles/hfp-design-system.css";
import { startCheckedWorkspace } from "@/services/diagnostics/startup-preflight";

void startCheckedWorkspace(() => import("./bootstrap-workspace").then((module) => module.startApplication()));
