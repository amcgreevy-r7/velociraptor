package notebook

import (
	"context"
	"errors"
	"os"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
)

func (self *NotebookStoreImpl) GetAvailableTimelines(notebook_id string) []string {
	path_manager := paths.NewNotebookPathManager(notebook_id)
	result := []string{}
	db, err := datastore.GetDB(self.config_obj)
	files, err := db.ListChildren(self.config_obj, path_manager.SuperTimelineDir())
	if err != nil {
		return nil
	}

	for _, f := range files {
		if !f.IsDir() {
			result = append(result, f.Base())
		}
	}
	return result
}

func (self *NotebookStoreImpl) GetAvailableDownloadFiles(
	notebook_id string) (*api_proto.AvailableDownloads, error) {

	download_path := paths.NewNotebookPathManager(notebook_id).
		HtmlExport("X").Dir()

	return reporting.GetAvailableDownloadFiles(self.config_obj, download_path)
}

func (self *NotebookStoreImpl) GetAvailableUploadFiles(notebook_id string) (
	*api_proto.AvailableDownloads, error) {
	result := &api_proto.AvailableDownloads{}

	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)
	file_store_factory := file_store.GetFileStore(self.config_obj)

	notebook, err := self.GetNotebook(notebook_id)
	if err != nil {
		return nil, err
	}

	for _, cell_metadata := range notebook.CellMetadata {
		cell_manager := notebook_path_manager.Cell(
			cell_metadata.CellId, cell_metadata.CurrentVersion)

		upload_directory := cell_manager.UploadsDir()

		err := api.Walk(file_store_factory, cell_manager.UploadsDir(),
			func(ps api.FSPathSpec, info os.FileInfo) error {

				// Build the vfs path by showing the relative path of
				// the path spec relative to the uploads directory in
				// the cell. Uploads may be nested in arbitrary paths.
				vfs_path_spec := path_specs.NewUnsafeFilestorePath(
					ps.Components()[len(upload_directory.Components()):]...).
					SetType(ps.Type())
				vfs_path := vfs_path_spec.AsClientPath()

				result.Files = append(result.Files, &api_proto.AvailableDownloadFile{
					Name: ps.Base(),
					Size: uint64(info.Size()),
					Date: info.ModTime().UTC().Format(time.RFC3339),
					Type: api.GetExtensionForFilestore(ps),
					Stats: &api_proto.ContainerStats{
						Components: ps.Components(),
						Type:       api.GetExtensionForFilestore(ps),
						VfsPath:    vfs_path,
					},
				})
				return nil
			})
		if err != nil {
			return nil, err
		}
	}

	attachment_directory := notebook_path_manager.AttachmentDirectory()
	attachment_directory_components := len(attachment_directory.Components())
	// Also include attachments
	items, _ := file_store_factory.ListDirectory(attachment_directory)
	for _, item := range items {
		ps := item.PathSpec()
		file_type := api.GetExtensionForFilestore(ps)

		// Build the vfs path by showing the relative path of
		// the path spec relative to the attachment
		vfs_path_spec := path_specs.NewUnsafeFilestorePath(
			ps.Components()[attachment_directory_components:]...).
			SetType(ps.Type())

		result.Files = append(result.Files, &api_proto.AvailableDownloadFile{
			Name: ps.Base(),
			Size: uint64(item.Size()),
			Date: item.ModTime().UTC().Format(time.RFC3339),
			Type: file_type,
			Stats: &api_proto.ContainerStats{
				Components: ps.Components(),
				Type:       file_type,
				VfsPath:    vfs_path_spec.AsClientPath(),
			},
		})
	}

	return result, nil
}

func (self *NotebookManager) RemoveNotebookAttachment(ctx context.Context,
	notebook_id string, components []string) error {
	return self.Store.RemoveAttachment(ctx, notebook_id, components)
}

func (self *NotebookStoreImpl) RemoveAttachment(ctx context.Context,
	notebook_id string, components []string) error {

	if len(components) == 0 {
		return errors.New("Attachment components empty")
	}

	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)
	attachment_path := path_specs.NewUnsafeFilestorePath(components...).
		SetType(api.PATH_TYPE_FILESTORE_ANY)
	if !path_specs.IsSubPath(
		notebook_path_manager.NotebookDirectory(),
		attachment_path) {
		return errors.New("Attachment must be within the notebook directory")
	}

	file_store_factory := file_store.GetFileStore(self.config_obj)
	return file_store_factory.Delete(attachment_path)
}
