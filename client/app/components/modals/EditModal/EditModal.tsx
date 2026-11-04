import { Link } from "react-router";
import { Modal } from "../Modal";
import AliasManager from "./AliasManager";

interface Props {
  type: string;
  id: number;
  open: boolean;
  setOpen: Function;
}

export default function EditModal({ open, setOpen, type, id }: Props) {
  type = type.toLowerCase();

  const handleClose = () => {
    setOpen(false);
  };

  return (
    <Modal maxW={1000} isOpen={open} onClose={handleClose}>
      <div className="flex flex-col items-start gap-6 w-full">
        <Link className="underline" to={`/filters?catalog=${type}&id=${id}`} onClick={handleClose}>Edit name</Link>
        <AliasManager id={id} type={type} />
        {type === "album" && (
          <>
            <Link className="underline" to={`/filters?catalog=album&id=${id}&edit=classification`} onClick={handleClose}>Edit Various Artists</Link>
          </>
        )}
        {type !== "artist" && <Link className="underline" to={`/filters?catalog=${type}&id=${id}&edit=credits`} onClick={handleClose}>Edit artist credits</Link>}
        <Link className="underline" to={`/filters?catalog=${type}&id=${id}&edit=identifier`} onClick={handleClose}>Edit MusicBrainz identifier</Link>
      </div>
    </Modal>
  );
}
