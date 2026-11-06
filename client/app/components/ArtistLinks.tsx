import React from "react";
import { Link } from "react-router";

type Artist = {
  id: number;
  name: string;
};

type ArtistLinksProps = {
  artists: Artist[];
};

const ArtistLinks: React.FC<ArtistLinksProps> = ({ artists }) => {
  return (
    <>
      {artists.map((artist, index) => (
        <span key={artist.id} className="color-fg-secondary">
          {artist.id > 0 ? <Link
            className="hover:text-(--color-fg-tertiary)"
            to={`/artist/${artist.id}`}
          >
            {artist.name}
          </Link> : artist.name}
          {index < artists.length - 1 ? ", " : ""}
        </span>
      ))}
    </>
  );
};

export default ArtistLinks;
