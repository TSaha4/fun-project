import type { CompanionState } from "./companion-state";

export function PandaCharacter({ state }: { state: CompanionState }) {
  return <div className={`companion-sprite companion-sprite--${state}`} aria-hidden="true">
    <svg className="panda-art" viewBox="0 0 120 132" role="presentation">
      <ellipse className="panda-shadow" cx="60" cy="124" rx="38" ry="6" />
      <path className="panda-black" d="M31 95c-9 5-13 12-19 11-7-2-7-10-2-14 5-4 12-3 16-11l12-9 16 8-8 18-9 11H27zM89 95c9 5 13 12 19 11 7-2 7-10 2-14-5-4-12-3-16-11L82 72l-16 8 8 18 9 11h10z" />
      <path className="panda-white" d="M39 77c-9 10-12 24-8 36 4 11 15 15 29 15s25-4 29-15c4-12 1-26-8-36z" />
      <path className="panda-black" d="M39 94c-6 9-7 20-2 28h13c-3-8-2-16 3-23zM81 94c6 9 7 20 2 28H70c3-8 2-16-3-23z" />
      <path className="panda-white" d="M40 84c3 7 9 10 20 10s17-3 20-10l-5-9H45z" />
      <path className="panda-ear" d="M24 33C17 18 18 7 25 5c9-2 18 8 23 20l-3 19zM96 33c7-15 6-26-1-28-9-2-18 8-23 20l3 19z" />
      <path className="panda-head" d="M20 45c0-22 17-37 40-37s40 15 40 37c0 25-15 42-40 42S20 70 20 45z" />
      <path className="panda-patch" d="M32 39c5-8 14-9 20-3 4 5 2 14-3 19-5 5-14 4-18-2-3-4-2-10 1-14zM68 36c6-6 15-5 20 3 3 4 4 10 1 14-4 6-13 7-18 2-5-5-7-14-3-19z" />
      <ellipse className="panda-eye" cx="43" cy="47" rx="3.3" ry="4.2" /><ellipse className="panda-eye" cx="77" cy="47" rx="3.3" ry="4.2" />
      <circle className="panda-glint" cx="44" cy="45" r="1.2" /><circle className="panda-glint" cx="78" cy="45" r="1.2" />
      <ellipse className="panda-muzzle" cx="60" cy="61" rx="14" ry="10" /><path className="panda-nose" d="M55 57c1-3 9-3 10 0l-5 4z" />
      <path className="panda-mouth" d="M60 61v4m0 0c-3 4-7 4-9 1m9-1c3 4 7 4 9 1" />
      <circle className="panda-blush" cx="34" cy="61" r="3" /><circle className="panda-blush" cx="86" cy="61" r="3" />
      <path className="panda-bamboo" d="M90 79l11-13m-9 6 4 4m0-9 4 4" />
    </svg>
    <span className="panda-caption">panda pal</span>
  </div>;
}
