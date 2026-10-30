import { Alert, App, Button, Popconfirm, Space, Table } from "antd";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router";
import { WorkshopEmptyState } from "@/components/workshop/workshop-card";
import { platformErrorMessage } from "@/lib/platform-account";
import { workshopSubmissionStatus } from "@/lib/workshop-media";
import { listWorkshopSubmissions, unlistWorkshopPublication, withdrawWorkshopVersion, workshopQueryKey } from "@/services/api/workshop";

export function MySubmissions() {
    const { message } = App.useApp();
    const client = useQueryClient();
    const submissions = useQuery({ queryKey: [...workshopQueryKey, "mine"], queryFn: ({ signal }) => listWorkshopSubmissions(signal) });
    const mutation = useMutation({
        mutationFn: (operation: () => Promise<unknown>) => operation(),
        onSuccess: async () => { message.success("已保存"); await client.invalidateQueries({ queryKey: workshopQueryKey }); },
        onError: async (error) => { message.error(platformErrorMessage(error)); await submissions.refetch(); },
    });
    return <div className="space-y-4">
        <div className="flex justify-end"><Button onClick={() => void submissions.refetch()}>刷新</Button></div>
        {submissions.isError && <Alert type="error" title={platformErrorMessage(submissions.error)} />}
        <Table rowKey={(item) => item.publication.id} dataSource={submissions.data?.items} loading={submissions.isFetching} scroll={{ x: 900 }} pagination={{ pageSize: 20 }} locale={{ emptyText: <WorkshopEmptyState /> }} columns={[
            { title: "作品", key: "title", render: (_, item) => item.latest.title },
            { title: "版本", key: "version", render: (_, item) => `版本 ${item.latest.versionNo}` },
            { title: "状态", key: "status", render: (_, item) => workshopSubmissionStatus(item.publication, item.latest) },
            { title: "审核意见", key: "note", render: (_, item) => <div className="max-w-sm whitespace-pre-wrap break-words">{item.latest.reviewNote}{item.publication.unlistReason && <p>下架原因：{item.publication.unlistReason}</p>}</div> },
            { title: "提交时间", key: "time", render: (_, item) => new Date(item.latest.createdAt).toLocaleString("zh-CN") },
            { title: "操作", key: "actions", render: (_, item) => <Space wrap>
                {item.publication.currentVersionId && <Link to={`/workshop/${item.publication.id}`}>查看</Link>}
                {item.latest.reviewStatus === "pending" && <Popconfirm title="撤回这次投稿？" onConfirm={() => mutation.mutate(() => withdrawWorkshopVersion(item.latest.id))}><Button size="small" disabled={mutation.isPending}>撤回</Button></Popconfirm>}
                {item.publication.status === "listed" && <Popconfirm title="下架后，同事将无法查看或复制这份作品。确定下架？" onConfirm={() => mutation.mutate(() => unlistWorkshopPublication(item.publication.id))}><Button size="small" disabled={mutation.isPending}>下架</Button></Popconfirm>}
            </Space> },
        ]} />
    </div>;
}
