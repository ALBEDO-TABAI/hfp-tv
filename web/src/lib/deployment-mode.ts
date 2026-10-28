export function isTeamDeployment(mode: string | undefined = import.meta.env.VITE_DEPLOYMENT_MODE) {
    return mode === "team";
}
