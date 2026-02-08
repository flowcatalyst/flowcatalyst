<?php

namespace FlowCatalyst\Generated\Endpoint;

class GrantPrincipalClientAccess extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\GrantClientAccessRequest $requestBody
     */
    public function __construct(string $id, ?\FlowCatalyst\Generated\Model\GrantClientAccessRequest $requestBody = null)
    {
        $this->id = $id;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return str_replace(['{id}'], [$this->id], '/api/admin/principals/{id}/client-access');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\GrantClientAccessRequest) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\GrantPrincipalClientAccessBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\GrantPrincipalClientAccessNotFoundException
     *
     * @return null
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (201 === $status) {
            return null;
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\GrantPrincipalClientAccessBadRequestException($response);
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\GrantPrincipalClientAccessNotFoundException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}