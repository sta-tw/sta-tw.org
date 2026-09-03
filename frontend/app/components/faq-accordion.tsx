"use client";

import Link from "next/link";
import { ChevronRight } from "lucide-react";
import type { ReactNode } from "react";
import { Accordion } from "radix-ui";
import styles from "./faq-accordion.module.css";

type FaqItem = {
    question: string;
    answer: ReactNode;
};

const faqItems: FaqItem[] = [
    {
        question: "特殊選才一定要競賽？",
        answer: "不一定。只要你有足以展現「特殊」之處的經歷，例如開發了上萬人使用的專案、創立公司，或在其他領域有長期且突出的表現，都可以嘗試申請特殊選才。"
    },
    {
        question: "沒有獎項可以申請？",
        answer: "可以。競賽獎項不是唯一證明方式，作品、專案、研究、創業或其他持續投入的經歷，也能用來呈現你的能力與發展潛力；實際資格仍應以各校系簡章為準。"
    },
    {
        question: "備審需要準備什麼？",
        answer: (
            <>
                備審資料每個學校、科系要求都不同。可以先到
                <Link href="/search" className="underline underline-offset-4 hover:text-ink">
                    簡章搜尋
                </Link>
                查看歷屆簡章，並參考學長姐經驗與備審準備方向。
            </>
        )
    },
    {
        question: "每間學校時程一樣？",
        answer: "不一樣。多數學校約在 10～11 月報名並審查備審資料、11～12 月面試、12 月至隔年 1 月公布正取名單，備取遞補則可能持續到 3 月；仍有學校早於或晚於上述時程，請以各校當年度簡章為準。"
    }
];

export default function FaqAccordion() {
    return (
        <Accordion.Root type="single" collapsible className="flex flex-col gap-5 sm:gap-7">
            {faqItems.map((item, index) => (
                <Accordion.Item
                    key={item.question}
                    value={`faq-${index + 1}`}
                    className="overflow-hidden rounded-[var(--radius-small)] bg-ink/5 text-ink"
                >
                    <Accordion.Header className="flex">
                        <Accordion.Trigger className="group flex min-h-16 w-full cursor-pointer items-center gap-4 px-5 py-4 text-left font-sans text-lg leading-relaxed transition-colors outline-none hover:bg-ink/5 focus-visible:ring-2 focus-visible:ring-ink/60 focus-visible:ring-inset sm:min-h-18 sm:gap-5 sm:px-8 sm:text-xl">
                            <ChevronRight
                                aria-hidden
                                className="h-5 w-5 shrink-0 stroke-[1.75] transition-transform duration-200 group-data-[state=open]:rotate-90 sm:h-6 sm:w-6"
                            />
                            <span>{item.question}</span>
                        </Accordion.Trigger>
                    </Accordion.Header>
                    <Accordion.Content className={styles.content}>
                        <div className="mr-5 ml-13 border-t border-ink/10 pt-4 pb-5 font-sans text-base leading-8 text-ink/75 sm:mr-8 sm:ml-19 sm:pt-5 sm:pb-6 sm:text-lg">
                            {item.answer}
                        </div>
                    </Accordion.Content>
                </Accordion.Item>
            ))}
        </Accordion.Root>
    );
}
