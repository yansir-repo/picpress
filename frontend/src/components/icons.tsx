import type { SVGProps } from "react";

type IconProps = SVGProps<SVGSVGElement>;

export function BrandMark(props: IconProps) {
  return (
    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true" {...props}>
      <path d="M4 8.5 12 4l8 4.5-8 4.5-8-4.5Z" fill="currentColor" opacity=".95" />
      <path
        d="M4 12.4 12 17l8-4.6"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
        strokeLinejoin="round"
        opacity=".55"
      />
      <path
        d="M4 16.2 12 20.8l8-4.6"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
        strokeLinejoin="round"
        opacity=".3"
      />
    </svg>
  );
}

export function PlusIcon(props: IconProps) {
  return (
    <svg viewBox="0 0 16 16" fill="none" aria-hidden="true" {...props}>
      <path d="M8 3.5v9M3.5 8h9" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  );
}

export function DownloadIcon(props: IconProps) {
  return (
    <svg viewBox="0 0 16 16" fill="none" aria-hidden="true" {...props}>
      <path
        d="M8 2.8v7.4m0 0 3-3m-3 3-3-3M3 12.5h10"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function CloseIcon(props: IconProps) {
  return (
    <svg viewBox="0 0 16 16" fill="none" aria-hidden="true" {...props}>
      <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  );
}

export function CompareIcon(props: IconProps) {
  return (
    <svg viewBox="0 0 16 16" fill="none" aria-hidden="true" {...props}>
      <rect
        x="2.2"
        y="3.2"
        width="11.6"
        height="9.6"
        rx="2"
        stroke="currentColor"
        strokeWidth="1.25"
      />
      <path d="M8 2v12" stroke="currentColor" strokeWidth="1.25" strokeDasharray="1.8 1.8" />
    </svg>
  );
}

export function DropIllustration() {
  return (
    <svg viewBox="0 0 220 170" fill="none">
      <defs>
        <linearGradient
          id="sunset"
          x1="66"
          y1="45"
          x2="172"
          y2="142"
          gradientUnits="userSpaceOnUse"
        >
          <stop stopColor="#FFC27D" />
          <stop offset="1" stopColor="#FF6B3D" />
        </linearGradient>
      </defs>
      <g transform="rotate(-7 110 88)">
        <rect x="43" y="31" width="134" height="108" rx="18" fill="#fff" stroke="#E4DED3" />
      </g>
      <g transform="rotate(4 110 88)">
        <rect x="45" y="34" width="134" height="108" rx="18" fill="#FFFDFA" stroke="#E7DED0" />
        <rect x="57" y="46" width="110" height="84" rx="11" fill="#FFF4E7" />
        <circle cx="137" cy="68" r="9" fill="#FFB579" />
        <path d="M57 119l30-34 20 22 14-14 26 26H57Z" fill="url(#sunset)" />
      </g>
      <g
        stroke="#1C1B19"
        strokeWidth="4"
        strokeLinecap="round"
        strokeLinejoin="round"
        opacity=".26"
      >
        <path d="M20 74h16M12 88h16M28 102h8" />
        <path d="M200 74h-16M208 88h-16M192 102h-8" />
      </g>
    </svg>
  );
}
