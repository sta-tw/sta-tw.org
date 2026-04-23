"use client";

import { ChevronLeft, ChevronRight } from "lucide-react";
import Image from "next/image";
import { useEffect, useEffectEvent, useState } from "react";
import { twMerge } from "tailwind-merge";
import IconButton from "./icon-button";

type Slide = {
  id: string;
  imageSrc: string;
  imageAlt: string;
};

type ArticlesCarouselProps = {
  slides: Slide[];
};

const AUTO_ADVANCE_MS = 5000;

export default function ArticlesCarousel({ slides }: ArticlesCarouselProps) {
  const [activeIndex, setActiveIndex] = useState(0);
  const [isPaused, setIsPaused] = useState(false);

  const goToSlide = (nextIndex: number) => {
    setActiveIndex((nextIndex + slides.length) % slides.length);
  };

  const showPrevious = () => {
    goToSlide(activeIndex - 1);
  };

  const showNext = () => {
    setActiveIndex((currentIndex) => (currentIndex + 1) % slides.length);
  };

  const autoAdvance = useEffectEvent(() => {
    showNext();
  });

  useEffect(() => {
    if (slides.length < 2 || isPaused) {
      return;
    }

    const intervalId = window.setInterval(() => {
      autoAdvance();
    }, AUTO_ADVANCE_MS);

    return () => {
      window.clearInterval(intervalId);
    };
  }, [isPaused, slides.length]);

  if (slides.length === 0) {
    return null;
  }

  return (
    <section
      aria-roledescription="carousel"
      aria-label="精選文章輪播"
      className="group relative overflow-hidden bg-[#163043]"
      onMouseEnter={() => setIsPaused(true)}
      onMouseLeave={() => setIsPaused(false)}
      onFocusCapture={() => setIsPaused(true)}
      onBlurCapture={() => setIsPaused(false)}
    >
      <div className="relative h-[15rem] sm:h-[18rem] lg:h-[22rem]">
        <ul
          className="flex h-full transition-transform duration-700 ease-out"
          style={{ transform: `translateX(-${activeIndex * 100}%)` }}
        >
          {slides.map((slide, index) => (
            <li
              key={slide.id}
              className="relative h-full min-w-full"
              aria-hidden={index !== activeIndex}
            >
              <Image
                src={slide.imageSrc}
                alt={slide.imageAlt}
                fill
                priority={index === 0}
                sizes="100vw"
                className="object-cover"
              />
              <div className="absolute inset-0 bg-[linear-gradient(180deg,rgba(9,23,35,0.06)_0%,rgba(9,23,35,0.18)_100%)]" />
            </li>
          ))}
        </ul>
      </div>

      {slides.length > 1 ? (
        <>
          <CarouselArrowButton
            direction="previous"
            label="上一張"
            onClick={showPrevious}
            className="left-4 sm:left-5 lg:left-6"
          />
          <CarouselArrowButton
            direction="next"
            label="下一張"
            onClick={showNext}
            className="right-4 sm:right-5 lg:right-6"
          />

          <div className="absolute inset-x-0 bottom-4 flex justify-center sm:bottom-5 lg:bottom-6">
            <div className="flex items-center gap-3 rounded-full bg-white/10 px-4 py-2 backdrop-blur-sm">
              {slides.map((slide, index) => (
                <button
                  key={slide.id}
                  type="button"
                  aria-label={`前往第 ${index + 1} 張`}
                  aria-pressed={index === activeIndex}
                  className={twMerge(
                    "h-4 w-4 cursor-pointer rounded-full bg-white/80 transition-all duration-200 hover:bg-white focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-white",
                    index === activeIndex && "scale-110 bg-[#f6c24d]"
                  )}
                  onClick={() => goToSlide(index)}
                />
              ))}
            </div>
          </div>
        </>
      ) : null}
    </section>
  );
}

type CarouselArrowButtonProps = {
  className?: string;
  direction: "previous" | "next";
  label: string;
  onClick: () => void;
};

function CarouselArrowButton({
  className,
  direction,
  label,
  onClick,
}: CarouselArrowButtonProps) {
  return (
    <IconButton
      type="button"
      label={label}
      icon={
        direction === "previous" ? (
          <ChevronLeft className="h-6 w-6 sm:h-8 sm:w-8 lg:h-9 lg:w-9" strokeWidth={2.75} />
        ) : (
          <ChevronRight className="h-6 w-6 sm:h-8 sm:w-8 lg:h-9 lg:w-9" strokeWidth={2.75} />
        )
      }
      className={twMerge(
        "absolute top-1/2 z-10 inline-flex h-12 w-12 -translate-y-1/2 items-center justify-center rounded-full bg-[#ffe287] text-[#2c2e35] shadow-[0_14px_30px_rgba(23,29,38,0.18)] transition-transform duration-200 hover:scale-[1.03] focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-[#ffe287] sm:h-14 sm:w-14 lg:h-16 lg:w-16",
        className
      )}
      onClick={onClick}
    />
  );
}
