package volumes

import "testing"

func TestValidateCreateNormalizesAbsoluteDestination(t *testing.T) {
	serviceID := "cc66f7be-8888-4b76-b592-1cba17bab058"
	input, err := validateCreate(CreateInput{
		ServiceID: &serviceID, Name: " App data ", Source: " app_data ", Destination: "/var/lib/app/",
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Name != "App data" || input.Source != "app_data" || input.Destination != "/var/lib/app" {
		t.Fatalf("normalized volume = %#v", input)
	}
}

func TestValidateCreateRequiresExactlyOneOwner(t *testing.T) {
	_, err := validateCreate(CreateInput{Name: "Data", Source: "data", Destination: "/data"})
	if err == nil {
		t.Fatal("expected missing owner validation error")
	}
}

func TestValidateDestinationRejectsRelativePathAndRoot(t *testing.T) {
	for _, destination := range []string{"data", "/"} {
		if validateDestination(destination) == nil {
			t.Fatalf("destination %q was accepted", destination)
		}
	}
}
