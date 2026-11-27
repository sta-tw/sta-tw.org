import type { ReactNode } from "react";
import ConfirmDialog from "../../components/confirm-dialog";

/** The 個資遮蔽 checklist every exchange / public portfolio must pass before
 * it goes to review. */
export default function SubmitReviewDialog({
    trigger,
    onConfirm
}: {
    trigger: ReactNode;
    onConfirm: () => void;
}) {
    return (
        <ConfirmDialog
            trigger={trigger}
            title="送審前，請確認個資已遮蔽"
            description={
                <div className="flex flex-col gap-4">
                    <p>
                        交換與公開備審會被其他人下載。
                        <strong className="font-medium text-danger">
                            「必須遮蔽」的項目未遮蔽，將不予通過審核。
                        </strong>
                    </p>
                    <div>
                        <p className="font-medium text-ink">必須遮蔽</p>
                        <ul className="mt-1 list-disc pl-5">
                            <li>身分證字號、居留證號</li>
                        </ul>
                    </div>
                    <div>
                        <p className="font-medium text-ink">建議遮蔽</p>
                        <ul className="mt-1 list-disc pl-5">
                            <li>本名</li>
                            <li>清晰可辨識本人的照片</li>
                        </ul>
                    </div>
                </div>
            }
            cancelLabel="再回去檢查"
            confirmLabel="確認送審"
            tone="primary"
            onConfirm={onConfirm}
        />
    );
}
